package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/export"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

func newExportCmd() *cobra.Command {
	var lockfilePath, format, outputPath, tag, cacheDir string

	cmd := &cobra.Command{
		Use:   "export --lockfile <lock.yaml> --format oci --output <file.tar>",
		Short: "Export a Strata environment as a container image",
		Long: `Convert the layers in a lockfile into a portable container image.

Supported formats:
  oci    OCI Image Layout tar archive (compatible with Docker, Podman, Apptainer)

Requires unsquashfs (from squashfs-tools) to unpack each layer.

The resulting archive can be loaded with:
  docker load -i output.tar
  podman load -i output.tar`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if lockfilePath == "" {
				return errors.New("--lockfile is required")
			}
			if outputPath == "" {
				return errors.New("--output is required")
			}
			if format != "oci" {
				return fmt.Errorf("unsupported format %q — only 'oci' is supported", format)
			}
			if cacheDir == "" {
				cacheDir = defaultCacheDir()
			}
			return runExport(context.Background(), lockfilePath, outputPath, tag, cacheDir)
		},
	}

	cmd.Flags().StringVar(&lockfilePath, "lockfile", "", "path to the lockfile (required)")
	cmd.Flags().StringVar(&format, "format", "oci", "export format (oci)")
	cmd.Flags().StringVar(&outputPath, "output", "", "output file path (required)")
	cmd.Flags().StringVar(&tag, "tag", "strata:latest", "image tag to embed in the OCI index")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "layer cache directory (default: ~/.cache/strata/layers)")
	return cmd
}

func runExport(ctx context.Context, lockfilePath, outputPath, tag, cacheDir string) error {
	// Read lockfile.
	data, err := os.ReadFile(lockfilePath)
	if err != nil {
		return fmt.Errorf("export: reading lockfile: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return fmt.Errorf("export: parsing lockfile: %w", err)
	}

	// Ensure cache dir exists.
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("export: creating cache dir: %w", err)
	}

	// Fetch layers to cache.
	layerPaths, err := fetchLayersToCache(ctx, lf, cacheDir)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	// Convert []overlay.LayerPath from fetchLayersToCache.
	var paths []overlay.LayerPath
	for _, lp := range layerPaths {
		paths = append(paths, overlay.LayerPath{
			ID:         lp.ID,
			SHA256:     lp.SHA256,
			Path:       lp.Path,
			MountOrder: lp.MountOrder,
		})
	}

	fmt.Printf("exporting %d layer(s) to %s...\n", len(paths), outputPath)
	if err := export.Export(ctx, &lf, paths, outputPath, tag); err != nil {
		return fmt.Errorf("export: %w", err)
	}

	// Report output size.
	fi, err := os.Stat(outputPath)
	if err == nil {
		fmt.Printf("exported: %s (%s)\n", outputPath, formatBytes(fi.Size()))
	} else {
		fmt.Printf("exported: %s\n", outputPath)
	}
	return nil
}
