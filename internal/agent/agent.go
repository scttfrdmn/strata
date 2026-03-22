// Package agent implements the Strata instance bootstrap sequence.
//
// The agent runs as a systemd service (strata-agent.service) at instance boot.
// It acquires the lockfile, verifies and fetches all layers, assembles the
// OverlayFS, writes environment config, and signals readiness. Any failure
// halts the instance — partial environments never run.
//
// All external dependencies are expressed as interfaces so the boot sequence
// can be fully tested on any platform without Linux syscalls or AWS access.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// LockfileSource provides the lockfile from instance metadata.
// Implementations check user-data, S3, and EC2 instance tags in priority order.
type LockfileSource interface {
	Acquire(ctx context.Context) (*spec.LockFile, error)
}

// LayerFetcher downloads a layer to the local cache if not already present.
// It is responsible for cache lookup and download; SHA256 verification is
// performed by the agent after Fetch returns.
type LayerFetcher interface {
	Fetch(ctx context.Context, layer spec.ResolvedLayer) (localPath string, err error)
}

// BundleFetcher downloads the Sigstore bundle JSON for a layer from the
// registry. Used alongside Verifier to perform cryptographic signature
// verification after layers are fetched. Implementations should return
// (nil, nil) when layer.Bundle is empty.
type BundleFetcher interface {
	FetchBundleJSON(ctx context.Context, layer spec.ResolvedLayer) ([]byte, error)
}

// ReadySignaler reports success or failure to the outside world.
// Implementations write EC2 instance tags, CloudWatch events, and call sd_notify.
type ReadySignaler interface {
	SignalReady(ctx context.Context, lockfile *spec.LockFile) error
	SignalFailed(ctx context.Context, reason error) error
}

// Mounter assembles the OverlayFS from a set of pulled layer paths.
// The default implementation calls overlay.Mount directly.
// FakeMounter is provided in fake.go for platform-neutral testing.
type Mounter interface {
	Mount(layers []overlay.LayerPath) (*overlay.Overlay, error)
}

// PackageInstaller installs resolved package sets into the mounted overlay.
// The mergedPath argument is the OverlayFS merged view (e.g. /strata/env).
// A nil PackageInstaller skips installation; callers should check whether
// the lockfile has any Packages before wiring a non-nil installer.
type PackageInstaller interface {
	Install(ctx context.Context, pkgs []spec.ResolvedPackageSet, mergedPath string) error
}

// Config holds all dependencies for the Agent.
// Verifier and BundleFetcher are optional; both must be non-nil to perform
// cosign bundle verification (only SHA256 content integrity is checked when
// either is nil). EnvRootDir defaults to "/" and is overridden in tests to a
// temp directory.
type Config struct {
	Source           LockfileSource
	Fetcher          LayerFetcher
	BundleFetcher    BundleFetcher  // optional; nil skips cosign bundle verification
	Verifier         trust.Verifier // optional; nil skips cosign bundle verification
	Signaler         ReadySignaler
	Mounter          Mounter          // optional; defaults to overlay.Mount
	PackageInstaller PackageInstaller // optional; nil skips package installation
	EnvRootDir       string           // defaults to "/" if empty
}

// Agent orchestrates the boot sequence for a Strata instance.
type Agent struct {
	cfg Config
}

// New creates a new Agent, validating that required config fields are present.
// If Mounter is nil it is defaulted to the real overlay.Mount implementation.
func New(cfg Config) (*Agent, error) {
	if cfg.Source == nil {
		return nil, fmt.Errorf("agent: Source is required")
	}
	if cfg.Fetcher == nil {
		return nil, fmt.Errorf("agent: Fetcher is required")
	}
	if cfg.Signaler == nil {
		return nil, fmt.Errorf("agent: Signaler is required")
	}
	if cfg.Mounter == nil {
		cfg.Mounter = overlayMounter{}
	}
	return &Agent{cfg: cfg}, nil
}

// overlayMounter is the production Mounter that delegates to overlay.Mount.
type overlayMounter struct{}

func (overlayMounter) Mount(layers []overlay.LayerPath) (*overlay.Overlay, error) {
	return overlay.Mount(layers)
}

// Run executes the 6-step boot sequence. On any failure, SignalFailed is called
// and the error is returned. On success, the overlay stays mounted until the
// process exits.
//
// The returned *BootMetrics contains timing data for each step. FetchBytes,
// CachedLayers, and DownloadedLayers are left at zero — callers that have
// access to the LayerFetcher's stats should populate them after Run returns.
//
//  1. Acquire lockfile from instance metadata.
//  2. Fetch all layers in parallel and verify SHA256.
//  3. Verify Sigstore bundles (when Verifier and BundleFetcher are configured).
//  4. Assemble the OverlayFS.
//  5. Write environment config files.
//  6. Signal readiness.
func (a *Agent) Run(ctx context.Context) (*BootMetrics, error) {
	started := time.Now()
	metrics := &BootMetrics{StartedAt: started}

	fail := func(err error) (*BootMetrics, error) {
		metrics.TotalMs = time.Since(started).Milliseconds()
		_ = a.cfg.Signaler.SignalFailed(ctx, err)
		return metrics, err
	}

	// Step 1: acquire lockfile.
	t0 := time.Now()
	lf, err := a.cfg.Source.Acquire(ctx)
	if err != nil {
		return fail(fmt.Errorf("agent: acquiring lockfile: %w", err))
	}
	metrics.LockfileMs = time.Since(t0).Milliseconds()
	metrics.LayerCount = len(lf.Layers)

	// Step 2: fetch and verify all layers in parallel.
	t0 = time.Now()
	layerPaths, err := a.fetchAndVerifyLayers(ctx, lf)
	if err != nil {
		return fail(fmt.Errorf("agent: fetching layers: %w", err))
	}
	metrics.FetchMs = time.Since(t0).Milliseconds()

	// Step 3: verify Sigstore bundles (requires fetched paths from step 2).
	if err := a.verifyBundles(ctx, lf, layerPaths); err != nil {
		return fail(fmt.Errorf("agent: verifying bundles: %w", err))
	}

	// Step 4: assemble the OverlayFS.
	t0 = time.Now()
	ov, err := a.cfg.Mounter.Mount(layerPaths)
	if err != nil {
		return fail(fmt.Errorf("agent: mounting overlay: %w", err))
	}
	metrics.MountMs = time.Since(t0).Milliseconds()

	// Step 4.5: install packages from lockfile.Packages (if any).
	if len(lf.Packages) > 0 && a.cfg.PackageInstaller != nil {
		if err := a.cfg.PackageInstaller.Install(ctx, lf.Packages, ov.MergedPath); err != nil {
			_ = ov.Cleanup()
			return fail(fmt.Errorf("agent: installing packages: %w", err))
		}
	}

	// Step 5: write environment config files.
	t0 = time.Now()
	rootDir := a.cfg.EnvRootDir
	if rootDir == "" {
		rootDir = "/"
	}
	if err := overlay.ConfigureEnvironment(lf, ov, rootDir); err != nil {
		_ = ov.Cleanup()
		return fail(fmt.Errorf("agent: configuring environment: %w", err))
	}
	metrics.ConfigureMs = time.Since(t0).Milliseconds()
	metrics.TotalMs = time.Since(started).Milliseconds()

	// Step 6: signal readiness. Overlay stays mounted on success.
	if err := a.cfg.Signaler.SignalReady(ctx, lf); err != nil {
		_ = ov.Cleanup()
		return metrics, fmt.Errorf("agent: signaling ready: %w", err)
	}

	return metrics, nil
}

// fetchAndVerifyLayers pulls all lockfile layers concurrently and verifies
// each layer's SHA256. The first error cancels all remaining fetches.
func (a *Agent) fetchAndVerifyLayers(ctx context.Context, lf *spec.LockFile) ([]overlay.LayerPath, error) {
	type result struct {
		path overlay.LayerPath
		err  error
	}

	if len(lf.Layers) == 0 {
		return nil, nil
	}

	results := make(chan result, len(lf.Layers))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, layer := range lf.Layers {
		layer := layer
		wg.Add(1)
		go func() {
			defer wg.Done()

			localPath, err := a.cfg.Fetcher.Fetch(ctx, layer)
			if err != nil {
				results <- result{err: fmt.Errorf("fetching layer %q: %w", layer.ID, err)}
				cancel()
				return
			}

			actual, err := sha256File(localPath)
			if err != nil {
				results <- result{err: fmt.Errorf("hashing layer %q: %w", layer.ID, err)}
				cancel()
				return
			}
			if actual != layer.SHA256 {
				results <- result{err: fmt.Errorf("layer %q SHA256 mismatch: manifest=%q file=%q",
					layer.ID, layer.SHA256, actual)}
				cancel()
				return
			}

			results <- result{path: overlay.LayerPath{
				ID:         layer.ID,
				SHA256:     layer.SHA256,
				Path:       localPath,
				MountOrder: layer.MountOrder,
			}}
		}()
	}

	wg.Wait()
	close(results)

	paths := make([]overlay.LayerPath, 0, len(lf.Layers))
	for r := range results {
		if r.err != nil {
			return nil, r.err
		}
		paths = append(paths, r.path)
	}
	return paths, nil
}

// verifyBundles verifies the Sigstore cosign bundle for each layer in
// parallel. Skipped when Verifier or BundleFetcher is nil, or when a layer
// has no Bundle field. The first verification failure cancels the rest.
func (a *Agent) verifyBundles(ctx context.Context, lf *spec.LockFile, paths []overlay.LayerPath) error {
	if a.cfg.Verifier == nil || a.cfg.BundleFetcher == nil {
		return nil
	}

	// Build map from layer ID to local sqfs path.
	pathByID := make(map[string]string, len(paths))
	for _, p := range paths {
		pathByID[p.ID] = p.Path
	}

	type result struct {
		err error
	}

	// Count layers that have both a local path and a bundle URI.
	var toVerify []spec.ResolvedLayer
	for _, layer := range lf.Layers {
		if _, ok := pathByID[layer.ID]; ok && layer.Bundle != "" {
			toVerify = append(toVerify, layer)
		}
	}
	if len(toVerify) == 0 {
		return nil
	}

	results := make(chan result, len(toVerify))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, layer := range toVerify {
		layer := layer
		localPath := pathByID[layer.ID]
		wg.Add(1)
		go func() {
			defer wg.Done()

			data, err := a.cfg.BundleFetcher.FetchBundleJSON(ctx, layer)
			if err != nil {
				results <- result{fmt.Errorf("fetching bundle for %q: %w", layer.ID, err)}
				cancel()
				return
			}
			if data == nil {
				results <- result{} // no bundle data — skip
				return
			}

			bundle, err := trust.ParseBundle(data)
			if err != nil {
				results <- result{fmt.Errorf("parsing bundle for %q: %w", layer.ID, err)}
				cancel()
				return
			}

			if err := a.cfg.Verifier.Verify(ctx, localPath, bundle); err != nil {
				results <- result{fmt.Errorf("verifying layer %q: %w", layer.ID, err)}
				cancel()
				return
			}

			results <- result{}
		}()
	}

	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			return r.err
		}
	}
	return nil
}

// sha256File returns the hex-encoded SHA256 of the named file's contents.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
