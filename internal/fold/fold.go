//go:build linux

package fold

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// MergeToLayer mounts the N layers in lf via OverlayFS, stages the merged
// tree into a new squashfs layer, signs it, and pushes it to cfg.Registry.
//
// layerPaths must already be fetched and verified (e.g. via fetchLayersToCache).
//
// This function is Linux-only because it relies on squashfs creation via
// mksquashfs and OverlayFS mounting.
func MergeToLayer(ctx context.Context, lf *spec.LockFile, layerPaths []overlay.LayerPath, cfg MergeConfig) (*spec.LayerManifest, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("fold: MergeToLayer: Name is required")
	}
	if cfg.Version == "" {
		return nil, fmt.Errorf("fold: MergeToLayer: Version is required")
	}

	// Resolve ABI and Arch from the lockfile if not overridden.
	abi, arch := cfg.ABI, cfg.Arch
	if (abi == "" || arch == "") && len(lf.Layers) > 0 {
		first := lf.Layers[0]
		if abi == "" {
			abi = first.ABI
		}
		if arch == "" {
			arch = first.Arch
		}
	}

	// Collect source SHA256s for provenance.
	foldedFrom := make([]string, 0, len(lf.Layers))
	for _, l := range lf.Layers {
		if l.SHA256 != "" {
			foldedFrom = append(foldedFrom, l.SHA256)
		}
	}

	manifest := &spec.LayerManifest{
		ID:             cfg.Name + "-" + cfg.Version + "-" + abi + "-" + arch,
		Name:           cfg.Name,
		Version:        cfg.Version,
		ABI:            abi,
		Arch:           arch,
		BuiltAt:        time.Now().UTC(),
		UserSelectable: true,
		CaptureSource:  "fold",
		FoldedFrom:     foldedFrom,
		Provides:       cfg.Provides,
		Requires:       cfg.Requires,
	}

	if cfg.DryRun {
		printDryRun(manifest, lf)
		return manifest, nil
	}

	if cfg.Registry == nil {
		return nil, fmt.Errorf("fold: MergeToLayer: Registry is required unless DryRun")
	}

	// Create temp workdir.
	workDir, err := os.MkdirTemp("", "strata-fold-*")
	if err != nil {
		return nil, fmt.Errorf("fold: creating work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	// Mount overlay.
	ovcfg := overlay.Config{
		LayersDir: filepath.Join(workDir, "layers"),
		RWDir:     filepath.Join(workDir, "rw"),
		MergedDir: filepath.Join(workDir, "env"),
	}
	ov, err := overlay.MountWithConfig(layerPaths, ovcfg)
	if err != nil {
		return nil, fmt.Errorf("fold: mounting overlay: %w", err)
	}
	defer ov.Cleanup() //nolint:errcheck

	// Stage merged tree into <workDir>/staging/<name>/<version>/.
	stagingRoot := filepath.Join(workDir, "staging")
	stagingDir := filepath.Join(stagingRoot, cfg.Name, cfg.Version)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return nil, fmt.Errorf("fold: creating staging dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "fold: staging merged tree for squashfs\n")
	if err := stageTree(ctx, ov.MergedPath, stagingDir); err != nil {
		return nil, fmt.Errorf("fold: staging tree: %w", err)
	}

	// Generate content manifest.
	contentManifest, err := build.GenerateContentManifest(stagingRoot, manifest.ID)
	if err != nil {
		return nil, fmt.Errorf("fold: generating content manifest: %w", err)
	}
	manifest.ContentManifest = contentManifest.Files

	// Create squashfs.
	sqfsFile, err := os.CreateTemp("", "strata-fold-*.sqfs")
	if err != nil {
		return nil, fmt.Errorf("fold: creating temp sqfs: %w", err)
	}
	sqfsPath := sqfsFile.Name()
	sqfsFile.Close()          //nolint:errcheck
	defer os.Remove(sqfsPath) //nolint:errcheck

	fmt.Fprintf(os.Stderr, "fold: creating squashfs\n")
	if err := build.CreateSquashfs(ctx, stagingRoot, sqfsPath); err != nil {
		return nil, fmt.Errorf("fold: creating squashfs: %w", err)
	}

	// Compute SHA256 and size.
	sha256hex, err := registry.SHA256HexFile(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("fold: hashing squashfs: %w", err)
	}
	stat, err := os.Stat(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("fold: stat squashfs: %w", err)
	}
	manifest.SHA256 = sha256hex
	manifest.Size = stat.Size()

	// Sign (optional).
	var bundleJSON []byte
	if !cfg.NoSign && cfg.KeyRef != "" {
		signer := &trust.CosignSigner{KeyRef: cfg.KeyRef}
		bundle, err := signer.Sign(ctx, sqfsPath, map[string]string{
			"strata.layer.name":    cfg.Name,
			"strata.layer.version": cfg.Version,
			"strata.capture":       "fold",
		})
		if err != nil {
			return nil, fmt.Errorf("fold: signing: %w", err)
		}
		bundleJSON, err = bundle.Marshal()
		if err != nil {
			return nil, fmt.Errorf("fold: marshaling bundle: %w", err)
		}
		if idx, ok := bundle.RekorLogIndex(); ok {
			manifest.RekorEntry = fmt.Sprintf("%d", idx)
		}
		if len(bundle.VerificationMaterial.TlogEntries) > 0 {
			if pk := bundle.VerificationMaterial.PublicKey; pk != nil && pk.Hint != "" {
				manifest.SignedBy = pk.Hint
			}
		}
		manifest.CosignVersion = trust.CosignToolVersion(ctx)
	}

	// Push to registry.
	fmt.Fprintf(os.Stderr, "fold: pushing layer to registry\n")
	if err := cfg.Registry.PushLayer(ctx, manifest, sqfsPath, bundleJSON); err != nil {
		return nil, fmt.Errorf("fold: pushing layer: %w", err)
	}

	return manifest, nil
}

// stageTree copies the overlay merged tree into stagingDir, preserving
// permissions and symlinks. Special files (devices, sockets) are skipped.
func stageTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(destPath, info.Mode()&os.ModePerm)
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(destPath)
			return os.Symlink(linkTarget, destPath)
		case info.Mode().IsRegular():
			return copyRegularFile(path, destPath, info.Mode()&os.ModePerm)
		default:
			return nil
		}
	})
}

// printDryRun prints the fold plan without executing.
func printDryRun(manifest *spec.LayerManifest, lf *spec.LockFile) {
	fmt.Fprintf(os.Stderr, "dry-run: fold plan\n")
	fmt.Fprintf(os.Stderr, "  name:        %s\n", manifest.Name)
	fmt.Fprintf(os.Stderr, "  version:     %s\n", manifest.Version)
	fmt.Fprintf(os.Stderr, "  abi:         %s\n", manifest.ABI)
	fmt.Fprintf(os.Stderr, "  arch:        %s\n", manifest.Arch)
	fmt.Fprintf(os.Stderr, "  id:          %s\n", manifest.ID)
	fmt.Fprintf(os.Stderr, "  source layers (%d):\n", len(lf.Layers))
	for _, l := range lf.Layers {
		sha := l.SHA256
		if len(sha) > 16 {
			sha = sha[:16]
		}
		fmt.Fprintf(os.Stderr, "    [%d] %s@%s (%s)\n", l.MountOrder, l.Name, l.Version, sha)
	}
	if len(manifest.Provides) > 0 {
		fmt.Fprintf(os.Stderr, "  provides: ")
		for i, p := range manifest.Provides {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			fmt.Fprintf(os.Stderr, "%s@%s", p.Name, p.Version)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}
	if len(manifest.Requires) > 0 {
		fmt.Fprintf(os.Stderr, "  requires: ")
		for i, r := range manifest.Requires {
			if i > 0 {
				fmt.Fprintf(os.Stderr, ", ")
			}
			fmt.Fprintf(os.Stderr, "%s", r.String())
		}
		fmt.Fprintf(os.Stderr, "\n")
	}
}
