package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/fold"
	"github.com/scttfrdmn/strata/spec"
)

func newFoldCmd() *cobra.Command {
	var (
		lockfilePath string
		name         string
		ver          string
		ejectDir     string
		abi          string
		arch         string
		noSign       bool
		key          string
		reg          string
		provides     string
		requires     string
		cacheDir     string
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "fold",
		Short: "Merge all layers in a lockfile into a single squashfs layer or plain directory",
		Long: `Merge all layers in a lockfile into one squashfs layer and push to a registry.
Use --eject to materialize to a plain directory instead.

Examples:

  # Dry-run fold (shows plan, no squashfs created):
  strata fold --lockfile env.lock.yaml --name my-stack --version 1.0.0 \
    --no-sign --dry-run

  # Fold to a local registry (Linux only):
  strata fold --lockfile env.lock.yaml --name my-stack --version 1.0.0 \
    --no-sign --registry file:///tmp/folded-registry

  # Fold to S3 registry (Linux only):
  strata fold --lockfile env.lock.yaml --name my-stack --version 1.0.0 \
    --registry s3://my-strata-bucket \
    --key awskms:///alias/strata-signing-key

  # Eject to a plain directory (Strata not required at runtime):
  strata fold --lockfile env.lock.yaml --eject /tmp/my-stack-root`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cacheDir == "" {
				cacheDir = defaultCacheDir()
			}
			return runFold(cmd.Context(), lockfilePath, name, ver, ejectDir, abi, arch, reg, key, provides, requires, cacheDir, noSign, dryRun)
		},
	}

	cmd.Flags().StringVar(&lockfilePath, "lockfile", "", "source lockfile (required)")
	cmd.Flags().StringVar(&name, "name", "", "name for the merged layer (required unless --eject)")
	cmd.Flags().StringVar(&ver, "version", "", "version for the merged layer (required unless --eject)")
	cmd.Flags().StringVar(&ejectDir, "eject", "", "materialize to a plain directory at DIR instead of creating a squashfs")
	cmd.Flags().StringVar(&abi, "abi", "", "override detected ABI (default: from lockfile base)")
	cmd.Flags().StringVar(&arch, "arch", "", "override detected arch (default: from lockfile base)")
	cmd.Flags().BoolVar(&noSign, "no-sign", false, "skip cosign signing")
	cmd.Flags().StringVar(&key, "key", "awskms:///alias/strata-signing-key",
		"cosign key file or KMS URI")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"destination registry URL (s3:// or file://); overrides STRATA_REGISTRY_URL")
	cmd.Flags().StringVar(&provides, "provides", "",
		"comma-separated capability=version pairs, e.g. python=3.11.11,gcc=13.2.0")
	cmd.Flags().StringVar(&requires, "requires", "",
		"comma-separated requirement strings, e.g. glibc@>=2.34")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "layer cache directory (default: defaultCacheDir())")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show plan without executing")

	_ = cmd.MarkFlagRequired("lockfile")
	return cmd
}

func runFold(ctx context.Context, lockfilePath, name, ver, ejectDir, abi, arch, reg, key, provides, requires, cacheDir string, noSign, dryRun bool) error {
	// Read and parse lockfile.
	data, err := os.ReadFile(lockfilePath)
	if err != nil {
		return fmt.Errorf("fold: reading lockfile: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return fmt.Errorf("fold: parsing lockfile: %w", err)
	}

	if len(lf.Layers) == 0 {
		return fmt.Errorf("fold: lockfile has no layers")
	}

	// Ensure cache directory exists.
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("fold: creating cache dir: %w", err)
	}

	// Eject mode.
	if ejectDir != "" {
		if dryRun {
			fmt.Fprintf(os.Stderr, "dry-run: fold --eject plan\n")
			fmt.Fprintf(os.Stderr, "  lockfile:    %s (%d layers)\n", lockfilePath, len(lf.Layers))
			fmt.Fprintf(os.Stderr, "  output dir:  %s\n", ejectDir)
			fmt.Fprintf(os.Stderr, "  root:        %s/root/\n", ejectDir)
			for _, l := range lf.Layers {
				sha := l.SHA256
				if len(sha) > 16 {
					sha = sha[:16]
				}
				fmt.Fprintf(os.Stderr, "  layer [%d]:  %s@%s (%s)\n", l.MountOrder, l.Name, l.Version, sha)
			}
			return nil
		}
		layerPaths, err := fetchLayersToCache(ctx, lf, cacheDir)
		if err != nil {
			return fmt.Errorf("fold: %w", err)
		}
		return fold.EjectToDir(ctx, &lf, layerPaths, fold.EjectConfig{
			OutputDir: ejectDir,
			CacheDir:  cacheDir,
		})
	}

	// Squashfs fold mode — name and version are required.
	if name == "" {
		return fmt.Errorf("fold: --name is required unless --eject is set")
	}
	if ver == "" {
		return fmt.Errorf("fold: --version is required unless --eject is set")
	}

	// Parse provides/requires.
	layerProvides, err := parseCapabilities(provides)
	if err != nil {
		return fmt.Errorf("fold: --provides: %w", err)
	}
	layerRequires, err := parseRequirements(requires)
	if err != nil {
		return fmt.Errorf("fold: --requires: %w", err)
	}

	// Dry-run uses MergeToLayer with DryRun=true (no layers fetched).
	if dryRun {
		cfg := fold.MergeConfig{
			Name:     name,
			Version:  ver,
			ABI:      abi,
			Arch:     arch,
			Provides: layerProvides,
			Requires: layerRequires,
			NoSign:   noSign,
			KeyRef:   key,
			DryRun:   true,
		}
		_, err := fold.MergeToLayer(ctx, &lf, nil, cfg)
		return err
	}

	if reg == "" {
		return fmt.Errorf("fold: --registry is required unless --dry-run or --eject is set")
	}

	// Fetch layers to cache.
	layerPaths, err := fetchLayersToCache(ctx, lf, cacheDir)
	if err != nil {
		return fmt.Errorf("fold: %w", err)
	}

	// Build registry client.
	client, err := newClientForURL(reg)
	if err != nil {
		return fmt.Errorf("fold: initializing registry: %w", err)
	}

	pushReg, ok := client.(build.PushRegistry)
	if !ok {
		return fmt.Errorf("fold: registry client does not support PushLayer")
	}

	cfg := fold.MergeConfig{
		Name:     name,
		Version:  ver,
		ABI:      abi,
		Arch:     arch,
		Provides: layerProvides,
		Requires: layerRequires,
		KeyRef:   key,
		NoSign:   noSign,
		Registry: pushReg,
	}

	manifest, err := fold.MergeToLayer(ctx, &lf, layerPaths, cfg)
	if err != nil {
		return fmt.Errorf("fold: %w", err)
	}
	return printBuildResult(manifest, false)
}
