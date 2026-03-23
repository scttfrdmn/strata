//go:build linux

// Package capture snapshots an installed prefix into a signed squashfs layer.
package capture

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/scan"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// PushRegistry is the narrow interface capture needs from the registry.
// Satisfied by *registry.S3Client and *registry.LocalClient.
type PushRegistry interface {
	PushLayer(ctx context.Context, manifest *spec.LayerManifest,
		sqfsPath string, bundleJSON []byte) error
}

// Config holds the configuration for a capture operation.
type Config struct {
	Name, Version, Prefix string       // all required
	ABI, Arch             string       // auto-detected if empty
	Normalize             bool         // rewrite paths (default: preserve-prefix)
	CaptureSource         string       // "lmod", "conda", "filesystem", ""
	OriginalPrefix        string       // = Prefix unless Normalize applied
	Signer                trust.Signer // nil = no signing
	Registry              PushRegistry // required unless DryRun
	DryRun                bool
	TempDir               string
	Provides              []spec.Capability
	Requires              []spec.Requirement
}

// Result is returned on a successful capture.
type Result struct {
	Manifest    *spec.LayerManifest
	SqfsPath    string
	RegistryURI string
}

// Capture snapshots an installed prefix into a signed squashfs layer and
// pushes it to the registry.
func Capture(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("capture: name is required")
	}
	if cfg.Version == "" {
		return nil, fmt.Errorf("capture: version is required")
	}
	if cfg.Prefix == "" && !cfg.DryRun {
		return nil, fmt.Errorf("capture: prefix is required")
	}
	if !cfg.DryRun && cfg.Registry == nil {
		return nil, fmt.Errorf("capture: registry is required unless --dry-run")
	}

	// Auto-detect ABI and arch.
	arch := cfg.Arch
	if arch == "" {
		arch = scan.CurrentArch()
	}
	abi := cfg.ABI
	if abi == "" {
		var err error
		abi, err = scan.CurrentABI()
		if err != nil {
			abi = "linux-gnu-2.34" // safe default
		}
	}

	originalPrefix := cfg.OriginalPrefix
	if originalPrefix == "" {
		originalPrefix = cfg.Prefix
	}

	// Build manifest skeleton.
	manifest := &spec.LayerManifest{
		ID:             cfg.Name + "-" + cfg.Version + "-" + abi + "-" + arch,
		Name:           cfg.Name,
		Version:        cfg.Version,
		ABI:            abi,
		Arch:           arch,
		BuiltAt:        time.Now().UTC(),
		UserSelectable: true,
		CaptureSource:  cfg.CaptureSource,
		OriginalPrefix: originalPrefix,
		Normalized:     cfg.Normalize,
		Provides:       cfg.Provides,
		Requires:       cfg.Requires,
	}

	// Dry-run: print manifest and return.
	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "dry-run: capture plan\n")
		fmt.Fprintf(os.Stderr, "  name:            %s\n", cfg.Name)
		fmt.Fprintf(os.Stderr, "  version:         %s\n", cfg.Version)
		fmt.Fprintf(os.Stderr, "  abi:             %s\n", abi)
		fmt.Fprintf(os.Stderr, "  arch:            %s\n", arch)
		fmt.Fprintf(os.Stderr, "  prefix:          %s\n", cfg.Prefix)
		fmt.Fprintf(os.Stderr, "  original_prefix: %s\n", originalPrefix)
		fmt.Fprintf(os.Stderr, "  id:              %s\n", manifest.ID)
		if cfg.CaptureSource != "" {
			fmt.Fprintf(os.Stderr, "  capture_source:  %s\n", cfg.CaptureSource)
		}
		return &Result{Manifest: manifest}, nil
	}

	// Create staging directory with <name>/<version>/ layout.
	tempDir := cfg.TempDir
	ownTempDir := false
	if tempDir == "" {
		var err error
		tempDir, err = os.MkdirTemp("", "strata-capture-*")
		if err != nil {
			return nil, fmt.Errorf("capture: creating temp dir: %w", err)
		}
		ownTempDir = true
	}
	if ownTempDir {
		defer os.RemoveAll(tempDir) //nolint:errcheck
	}

	stagingDir := filepath.Join(tempDir, "staging")
	layerDir := filepath.Join(stagingDir, cfg.Name, cfg.Version)
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		return nil, fmt.Errorf("capture: creating staging dir: %w", err)
	}

	srcDir := cfg.Prefix
	if cfg.Normalize {
		// Copy prefix and rewrite paths.
		copyDir := filepath.Join(tempDir, "normalized")
		if err := copyDirRecursive(cfg.Prefix, copyDir); err != nil {
			return nil, fmt.Errorf("capture: copying prefix for normalization: %w", err)
		}
		newPrefix := "/" + cfg.Name + "/" + cfg.Version
		modified, warnings, err := NormalizePaths(ctx, copyDir, cfg.Prefix, newPrefix)
		if err != nil {
			return nil, fmt.Errorf("capture: normalizing paths: %w", err)
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "normalized %d files\n", modified)
		srcDir = copyDir
	}

	// Use a symlink: staging/<name>/<version> → srcDir so mksquashfs
	// produces the correct /<name>/<version>/... layout.
	if err := os.Remove(layerDir); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("capture: removing staging layer dir: %w", err)
	}
	if err := os.Symlink(srcDir, layerDir); err != nil {
		return nil, fmt.Errorf("capture: symlinking prefix: %w", err)
	}

	// Generate content manifest.
	contentManifest, err := build.GenerateContentManifest(stagingDir, manifest.ID)
	if err != nil {
		return nil, fmt.Errorf("capture: generating content manifest: %w", err)
	}
	manifest.ContentManifest = contentManifest.Files

	// Create squashfs.
	sqfsFile, err := os.CreateTemp("", "strata-capture-*.sqfs")
	if err != nil {
		return nil, fmt.Errorf("capture: creating temp sqfs: %w", err)
	}
	sqfsPath := sqfsFile.Name()
	sqfsFile.Close()          //nolint:errcheck
	defer os.Remove(sqfsPath) //nolint:errcheck

	if err := build.CreateSquashfs(ctx, stagingDir, sqfsPath); err != nil {
		return nil, fmt.Errorf("capture: creating squashfs: %w", err)
	}

	// SHA256 and stat.
	sha256hex, err := registry.SHA256HexFile(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("capture: hashing squashfs: %w", err)
	}
	stat, err := os.Stat(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("capture: stat squashfs: %w", err)
	}
	manifest.SHA256 = sha256hex
	manifest.Size = stat.Size()

	// Sign if signer provided.
	var bundleJSON []byte
	if cfg.Signer != nil {
		bundle, err := cfg.Signer.Sign(ctx, sqfsPath, map[string]string{
			"strata.layer.name":    cfg.Name,
			"strata.layer.version": cfg.Version,
		})
		if err != nil {
			return nil, fmt.Errorf("capture: signing: %w", err)
		}
		bundleJSON, err = bundle.Marshal()
		if err != nil {
			return nil, fmt.Errorf("capture: marshaling bundle: %w", err)
		}
		if idx, ok := bundle.RekorLogIndex(); ok {
			manifest.RekorEntry = fmt.Sprintf("%d", idx)
		}
	}

	// Push to registry.
	if err := cfg.Registry.PushLayer(ctx, manifest, sqfsPath, bundleJSON); err != nil {
		return nil, fmt.Errorf("capture: pushing layer: %w", err)
	}

	return &Result{
		Manifest: manifest,
		SqfsPath: sqfsPath,
	}, nil
}

// copyDirRecursive copies srcDir to dstDir recursively.
func copyDirRecursive(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)

		if info.IsDir() {
			return os.MkdirAll(dst, info.Mode())
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close() //nolint:errcheck

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close() //nolint:errcheck

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Close()
	})
}
