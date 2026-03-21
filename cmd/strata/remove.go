package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func newRemoveCmd() *cobra.Command {
	var arch, abi, reg string
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "remove <name[@version]>",
		Short: "Remove a layer (or specific version) from the S3 registry",
		Long: `Permanently remove one or more layer versions from the S3 registry.

By default, removal is blocked if any stored lockfile references the layer.
Use --force to override the reference check.

Examples:
  strata remove python@3.11.11
  strata remove python@3.11.11 --arch x86_64
  strata remove python --dry-run
  strata remove python@3.11.11 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if reg == "" {
				return fmt.Errorf("--registry (or STRATA_REGISTRY_URL) is required")
			}

			name, version := parseNameVersion(args[0])

			c, err := registry.NewS3Client(reg)
			if err != nil {
				return fmt.Errorf("registry: %w", err)
			}

			ctx := context.Background()

			// Find matching manifests.
			manifests, err := c.ListLayers(ctx, name, arch, abi)
			if err != nil {
				return fmt.Errorf("listing layers: %w", err)
			}

			// Filter by exact version if one was specified.
			if version != "" {
				manifests = filterByVersion(manifests, version)
			}

			if len(manifests) == 0 {
				fmt.Println("no layers found")
				return nil
			}

			// Reference check — refuse if any stored lockfile references these layers.
			if !force {
				if err := checkLockfileRefs(ctx, c, manifests); err != nil {
					return err
				}
			}

			// Dry-run: print what would be removed.
			if dryRun {
				printRemovalPlan(manifests)
				return nil
			}

			// Delete each layer.
			var totalBytes int64
			for _, m := range manifests {
				if err := c.DeleteLayer(ctx, m); err != nil {
					return fmt.Errorf("deleting %s/%s: %w", m.Name, m.Version, err)
				}
				totalBytes += m.Size
				fmt.Printf("removed %s/%s (%s/%s)\n", m.Name, m.Version, m.Arch, m.ABI)
			}

			// Rebuild the index.
			if err := c.RebuildIndex(ctx); err != nil {
				return fmt.Errorf("rebuilding index: %w", err)
			}

			fmt.Printf("\n%d layer(s) removed (%s freed)\n", len(manifests), formatBytes(totalBytes))
			return nil
		},
	}

	cmd.Flags().StringVar(&arch, "arch", "", "filter by architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&abi, "abi", "", "filter by ABI (e.g. linux-gnu-2.34)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be removed without deleting")
	cmd.Flags().BoolVar(&force, "force", false, "remove even if referenced by stored lockfiles")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL (e.g. s3://my-strata-bucket); overrides STRATA_REGISTRY_URL")
	return cmd
}

// parseNameVersion splits "name@version" into (name, version).
// If no "@" is present, version is returned as "".
func parseNameVersion(arg string) (name, version string) {
	parts := strings.SplitN(arg, "@", 2)
	name = parts[0]
	if len(parts) == 2 {
		version = parts[1]
	}
	return
}

// filterByVersion returns only manifests whose Version matches exactly.
func filterByVersion(manifests []*spec.LayerManifest, version string) []*spec.LayerManifest {
	var result []*spec.LayerManifest
	for _, m := range manifests {
		if m.Version == version {
			result = append(result, m)
		}
	}
	return result
}

// checkLockfileRefs fetches all stored lockfiles and returns an error if any
// of the manifests are referenced.
func checkLockfileRefs(ctx context.Context, c *registry.S3Client, manifests []*spec.LayerManifest) error {
	// Build a set of IDs and SHA256s to check against.
	type key struct{ id, sha256 string }
	target := make(map[key]bool)
	for _, m := range manifests {
		target[key{id: m.ID, sha256: m.SHA256}] = true
	}

	records, err := c.ListLockfiles(ctx)
	if err != nil {
		return fmt.Errorf("listing lockfiles: %w", err)
	}

	type ref struct {
		key     string
		profile string
		date    string
	}
	var refs []ref
	for _, r := range records {
		for _, layer := range r.LockFile.Layers {
			if target[key{id: layer.ID, sha256: layer.SHA256}] {
				date := r.LockFile.ResolvedAt.Format("2006-01-02")
				refs = append(refs, ref{key: r.Key, profile: r.LockFile.ProfileName, date: date})
				break // one ref per lockfile is enough
			}
		}
	}

	if len(refs) == 0 {
		return nil
	}

	// Print the warning table and return an error.
	names := make([]string, 0, len(manifests))
	for _, m := range manifests {
		names = append(names, m.Name+"/"+m.Version)
	}
	fmt.Fprintf(os.Stderr, "strata: refusing to remove — %s is referenced by stored lockfile(s):\n\n",
		strings.Join(names, ", "))
	fmt.Fprintf(os.Stderr, "  %-44s  %-24s  %s\n", "LOCKFILE", "PROFILE", "RESOLVED")
	fmt.Fprintf(os.Stderr, "  %-44s  %-24s  %s\n",
		"--------------------------------------------",
		"------------------------", "----------")
	for _, r := range refs {
		fmt.Fprintf(os.Stderr, "  %-44s  %-24s  %s\n", r.key, r.profile, r.date)
	}
	fmt.Fprintf(os.Stderr, "\nuse --force to remove anyway (those lockfiles will no longer resolve)\n")
	return fmt.Errorf("layer(s) are referenced by %d lockfile(s); use --force to override", len(refs))
}

// printRemovalPlan prints what would be removed without deleting anything.
func printRemovalPlan(manifests []*spec.LayerManifest) {
	var totalBytes int64
	for _, m := range manifests {
		fmt.Printf("would remove %s/%s (%s/%s, %s)\n",
			m.Name, m.Version, m.Arch, m.ABI, formatBytes(m.Size))
		totalBytes += m.Size
	}
	fmt.Printf("\n%d layer(s) would be removed (%s freed)\n", len(manifests), formatBytes(totalBytes))
}

// formatBytes returns a human-readable byte count (e.g. "14.2 MB").
func formatBytes(n int64) string {
	const (
		mb = 1 << 20
		gb = 1 << 30
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
