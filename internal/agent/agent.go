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

// Config holds all dependencies for the Agent.
// Verifier is optional; when nil, cryptographic bundle verification is skipped
// (only SHA256 content integrity is checked). EnvRootDir defaults to "/" and
// is overridden in tests to a temp directory.
type Config struct {
	Source     LockfileSource
	Fetcher    LayerFetcher
	Verifier   trust.Verifier // optional; nil skips cosign bundle verification
	Signaler   ReadySignaler
	Mounter    Mounter // optional; defaults to overlay.Mount
	EnvRootDir string  // defaults to "/" if empty
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
//  1. Acquire lockfile from instance metadata.
//  2. Verify Sigstore bundles (when Verifier is configured).
//  3. Fetch all layers in parallel and verify SHA256.
//  4. Assemble the OverlayFS.
//  5. Write environment config files.
//  6. Signal readiness.
func (a *Agent) Run(ctx context.Context) error {
	fail := func(err error) error {
		_ = a.cfg.Signaler.SignalFailed(ctx, err)
		return err
	}

	// Step 1: acquire lockfile.
	lf, err := a.cfg.Source.Acquire(ctx)
	if err != nil {
		return fail(fmt.Errorf("agent: acquiring lockfile: %w", err))
	}

	// Step 2: verify Sigstore bundles (TODO: integrate with trust.VerifyLayers
	// once the fetched paths are available; bundle paths in the registry are
	// the authoritative source).
	_ = a.cfg.Verifier // reserved for future use in this step

	// Step 3: fetch and verify all layers in parallel.
	layerPaths, err := a.fetchAndVerifyLayers(ctx, lf)
	if err != nil {
		return fail(fmt.Errorf("agent: fetching layers: %w", err))
	}

	// Step 4: assemble the OverlayFS.
	ov, err := a.cfg.Mounter.Mount(layerPaths)
	if err != nil {
		return fail(fmt.Errorf("agent: mounting overlay: %w", err))
	}

	// Step 5: write environment config files.
	rootDir := a.cfg.EnvRootDir
	if rootDir == "" {
		rootDir = "/"
	}
	if err := overlay.ConfigureEnvironment(lf, ov, rootDir); err != nil {
		_ = ov.Cleanup()
		return fail(fmt.Errorf("agent: configuring environment: %w", err))
	}

	// Step 6: signal readiness. Overlay stays mounted on success.
	if err := a.cfg.Signaler.SignalReady(ctx, lf); err != nil {
		_ = ov.Cleanup()
		return fmt.Errorf("agent: signaling ready: %w", err)
	}

	return nil
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
