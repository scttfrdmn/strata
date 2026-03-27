package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/packages"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/spec"
)

func newUpdateCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "update <profile.yaml>",
		Short: "Re-resolve a profile and show what changed",
		Long: `Re-resolve a profile to the latest matching registry versions,
show a diff against the current lockfile, and write the updated lockfile.

Like 'strata freeze' but loads the existing lockfile as a baseline so you
can review exactly what changed before committing the update.

If the lockfile is already up to date, prints "Already up to date." and
exits without writing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runUpdate(args[0], output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output lockfile path (default: <profile-basename>.lock.yaml)")
	return cmd
}

func runUpdate(profilePath, outputFlag string) error {
	profile := loadProfile(profilePath)
	reg := buildFederatedClient(profile.Registries)
	probeClient := buildProbeClient()

	r, err := resolver.New(resolver.Config{
		Registry:      reg,
		Probe:         probeClient,
		StrataVersion: version,
		Warnings:      os.Stderr,
	})
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	newLF, err := r.Resolve(context.Background(), profile)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if !newLF.IsFrozen() {
		var missing []string
		for _, layer := range newLF.Layers {
			if layer.SHA256 == "" {
				missing = append(missing, layer.ID)
			}
		}
		fmt.Fprintf(os.Stderr, //nolint:errcheck
			"strata update: lockfile is not frozen — layers must be built and pushed to the registry first\n")
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "missing SHA256 for: %s (%d layer", joinNames(missing), len(missing)) //nolint:errcheck
			if len(missing) != 1 {
				fmt.Fprint(os.Stderr, "s") //nolint:errcheck
			}
			fmt.Fprintln(os.Stderr, ")") //nolint:errcheck
		}
		return errors.New("") // already printed
	}

	// Resolve package manager entries (pip/conda/cran) to pinned versions.
	if len(profile.Packages) > 0 {
		resolved, err := packages.ResolveAll(context.Background(), profile.Packages)
		if err != nil {
			return fmt.Errorf("update: resolving packages: %w", err)
		}
		newLF.Packages = resolved
	}

	outPath := resolveOutputPath(profilePath, outputFlag, ".lock.yaml")

	// Load existing lockfile for diff (best effort — skip if not found).
	var oldLF *spec.LockFile
	if existing, err := spec.ParseLockFile(outPath); err == nil {
		oldLF = existing
	}

	if oldLF != nil {
		hasDiff := printUpdateDiff(oldLF, newLF)
		if !hasDiff {
			fmt.Println("Already up to date.")
			return nil
		}
	}

	if err := writeYAML(outPath, newLF); err != nil {
		return err
	}
	fmt.Printf("updated: %s\n", outPath)
	return nil
}

// printUpdateDiff prints a human-readable diff between old and new lockfiles
// to stderr. Returns true if any differences were found.
func printUpdateDiff(old, new *spec.LockFile) bool {
	hasDiff := false

	// EnvironmentID
	id1 := old.EnvironmentID()
	id2 := new.EnvironmentID()
	if id1 != id2 {
		hasDiff = true
		switch {
		case id1 == "":
			fmt.Fprintf(os.Stderr, "EnvironmentID: (unfrozen) → %s\n", id2) //nolint:errcheck
		case id2 == "":
			fmt.Fprintf(os.Stderr, "EnvironmentID: %s → (unfrozen)\n", id1) //nolint:errcheck
		default:
			fmt.Fprintf(os.Stderr, "EnvironmentID: %s → %s\n", id1, id2) //nolint:errcheck
		}
	}

	// Base
	if diffs := diffBase(old, new); len(diffs) > 0 {
		hasDiff = true
		fmt.Fprintln(os.Stderr, "\nBase:") //nolint:errcheck
		for _, d := range diffs {
			fmt.Fprintln(os.Stderr, "  "+d) //nolint:errcheck
		}
	}

	// Layers
	added, removed, changed, unchanged := diffLayers(old.Layers, new.Layers)
	if len(added)+len(removed)+len(changed) > 0 {
		hasDiff = true
		fmt.Fprintf(os.Stderr, "\nLayers (%d changed, %d added, %d removed):\n", //nolint:errcheck
			len(changed), len(added), len(removed))
		for _, l := range removed {
			fmt.Fprintf(os.Stderr, "  - %s/%s\n", l.Name, l.Version) //nolint:errcheck
		}
		for _, pair := range changed {
			if pair[0].Version != pair[1].Version {
				fmt.Fprintf(os.Stderr, "  ~ %s/%s → %s/%s\n", pair[0].Name, pair[0].Version, pair[1].Name, pair[1].Version) //nolint:errcheck
			} else {
				fmt.Fprintf(os.Stderr, "  ~ %s/%s (content changed)\n", pair[0].Name, pair[0].Version) //nolint:errcheck
			}
		}
		for _, l := range added {
			fmt.Fprintf(os.Stderr, "  + %s/%s\n", l.Name, l.Version) //nolint:errcheck
		}
	} else if len(unchanged) > 0 && !hasDiff {
		// Print unchanged summary only when there's nothing else to show.
		fmt.Fprintf(os.Stderr, "\nLayers (%d unchanged):\n", len(unchanged)) //nolint:errcheck
		for _, l := range unchanged {
			fmt.Fprintf(os.Stderr, "  = %s/%s\n", l.Name, l.Version) //nolint:errcheck
		}
	}

	// Env
	if diffs := diffStringMap(old.Env, new.Env); len(diffs) > 0 {
		hasDiff = true
		fmt.Fprintln(os.Stderr, "\nEnv:") //nolint:errcheck
		for _, d := range diffs {
			fmt.Fprintln(os.Stderr, "  "+d) //nolint:errcheck
		}
	}

	// Packages
	if diffs := diffPackages(old.Packages, new.Packages); len(diffs) > 0 {
		hasDiff = true
		fmt.Fprintln(os.Stderr, "\nPackages:") //nolint:errcheck
		for _, d := range diffs {
			fmt.Fprintln(os.Stderr, "  "+d) //nolint:errcheck
		}
	}

	return hasDiff
}
