package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/packages"
	"github.com/scttfrdmn/strata/internal/resolver"
)

func newFreezeCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "freeze <profile.yaml>",
		Short: "Resolve and require all layers to be SHA256-pinned",
		Long: `Resolve a profile identically to "strata resolve", then verify that
every layer in the lockfile has a SHA256 hash. If any layers are missing
SHA256s they must be built and pushed to the registry first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profile := loadProfile(args[0])
			reg := buildFederatedClient(profile.Registries)
			probeClient := buildProbeClient()

			r, err := resolver.New(resolver.Config{
				Registry:      reg,
				Probe:         probeClient,
				StrataVersion: version,
				Warnings:      os.Stderr,
			})
			if err != nil {
				return fmt.Errorf("freeze: %w", err)
			}

			lf, err := r.Resolve(context.Background(), profile)
			if err != nil {
				return fmt.Errorf("freeze: %w", err)
			}

			if !lf.IsFrozen() {
				var missing []string
				for _, layer := range lf.Layers {
					if layer.SHA256 == "" {
						missing = append(missing, layer.ID)
					}
				}
				fmt.Fprintf(os.Stderr, //nolint:errcheck
					"strata freeze: lockfile is not frozen — layers must be built and pushed to the registry before freezing\n")
				if len(missing) > 0 {
					fmt.Fprintf(os.Stderr, "missing SHA256 for: %s (%d layer", joinNames(missing), len(missing)) //nolint:errcheck
					if len(missing) != 1 {
						fmt.Fprint(os.Stderr, "s") //nolint:errcheck
					}
					fmt.Fprintln(os.Stderr, ")") //nolint:errcheck
				}
				return errors.New("") // already printed; suppress double-print in main
			}

			// Resolve package manager entries (pip/conda/cran) to pinned versions.
			if len(profile.Packages) > 0 {
				resolved, err := packages.ResolveAll(context.Background(), profile.Packages)
				if err != nil {
					return fmt.Errorf("freeze: resolving packages: %w", err)
				}
				lf.Packages = resolved
			}

			outPath := resolveOutputPath(args[0], output, ".lock.yaml")
			if err := writeYAML(outPath, lf); err != nil {
				return err
			}
			fmt.Printf("frozen: %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "o", "o", "", "output lockfile path (default: <profile-basename>.lock.yaml)")
	return cmd
}

// joinNames joins a slice of strings with ", " for human-readable output.
func joinNames(names []string) string {
	result := ""
	for i, n := range names {
		if i > 0 {
			result += ", "
		}
		result += n
	}
	return result
}
