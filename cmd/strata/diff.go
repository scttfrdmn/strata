package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/spec"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <lock1.yaml> <lock2.yaml>",
		Short: "Compare two lockfiles",
		Long: `Show differences between two lockfiles: added, removed, and changed
layers, base AMI, environment variables, and package pins.

Exit code 0 means no differences. Exit code 1 means differences found.
Exit code 2 means an error occurred.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDiff(args[0], args[1])
		},
	}
}

func runDiff(path1, path2 string) error {
	lf1, err := spec.ParseLockFile(path1)
	if err != nil {
		return fmt.Errorf("diff: reading %s: %w", path1, err)
	}
	lf2, err := spec.ParseLockFile(path2)
	if err != nil {
		return fmt.Errorf("diff: reading %s: %w", path2, err)
	}

	hasDiff := false

	// EnvironmentID
	id1 := lf1.EnvironmentID()
	id2 := lf2.EnvironmentID()
	if id1 != id2 {
		hasDiff = true
		switch {
		case id1 == "" && id2 == "":
			fmt.Println("EnvironmentID: (unfrozen) → (unfrozen)")
		case id1 == "":
			fmt.Printf("EnvironmentID: (unfrozen) → %s\n", id2)
		case id2 == "":
			fmt.Printf("EnvironmentID: %s → (unfrozen)\n", id1)
		default:
			fmt.Printf("EnvironmentID: %s → %s\n", id1, id2)
		}
	}

	// Base
	baseDiffs := diffBase(lf1, lf2)
	if len(baseDiffs) > 0 {
		hasDiff = true
		fmt.Println("\nBase:")
		for _, d := range baseDiffs {
			fmt.Println(" ", d)
		}
	}

	// Layers
	added, removed, changed, unchanged := diffLayers(lf1.Layers, lf2.Layers)
	if len(added)+len(removed)+len(changed) > 0 {
		hasDiff = true
		fmt.Printf("\nLayers (%d changed, %d added, %d removed):\n",
			len(changed), len(added), len(removed))
		for _, l := range removed {
			fmt.Printf("  - %s/%s\n", l.Name, l.Version)
		}
		for _, pair := range changed {
			if pair[0].Version != pair[1].Version {
				fmt.Printf("  ~ %s/%s → %s/%s\n", pair[0].Name, pair[0].Version, pair[1].Name, pair[1].Version)
			} else {
				fmt.Printf("  ~ %s/%s (content changed)\n", pair[0].Name, pair[0].Version)
			}
		}
		for _, l := range added {
			fmt.Printf("  + %s/%s\n", l.Name, l.Version)
		}
	} else if len(unchanged) > 0 {
		fmt.Printf("\nLayers (%d unchanged):\n", len(unchanged))
		for _, l := range unchanged {
			fmt.Printf("  = %s/%s\n", l.Name, l.Version)
		}
	}

	// Env
	envDiffs := diffStringMap(lf1.Env, lf2.Env)
	if len(envDiffs) > 0 {
		hasDiff = true
		fmt.Println("\nEnv:")
		for _, d := range envDiffs {
			fmt.Println(" ", d)
		}
	}

	// Packages
	pkgDiffs := diffPackages(lf1.Packages, lf2.Packages)
	if len(pkgDiffs) > 0 {
		hasDiff = true
		fmt.Println("\nPackages:")
		for _, d := range pkgDiffs {
			fmt.Println(" ", d)
		}
	}

	if !hasDiff {
		fmt.Println("No differences found.")
		return nil
	}

	// Return a sentinel that suppresses the default error message in main.
	return errDiffFound
}

// errDiffFound is returned when strata diff finds differences.
// It satisfies the error interface but prints nothing (main prints it only if non-empty).
var errDiffFound = diffError{}

type diffError struct{}

func (diffError) Error() string { return "" }

// diffBase compares the base fields of two lockfiles.
func diffBase(lf1, lf2 *spec.LockFile) []string {
	var diffs []string
	if lf1.Base.AMIID != lf2.Base.AMIID {
		diffs = append(diffs, fmt.Sprintf("AMIID: %s → %s", lf1.Base.AMIID, lf2.Base.AMIID))
	}
	if lf1.Base.AMISHA256 != lf2.Base.AMISHA256 && (lf1.Base.AMISHA256 != "" || lf2.Base.AMISHA256 != "") {
		diffs = append(diffs, fmt.Sprintf("AMISHA256: %s → %s", lf1.Base.AMISHA256, lf2.Base.AMISHA256))
	}
	if lf1.Base.Capabilities.ABI != lf2.Base.Capabilities.ABI {
		diffs = append(diffs, fmt.Sprintf("ABI: %s → %s", lf1.Base.Capabilities.ABI, lf2.Base.Capabilities.ABI))
	}
	return diffs
}

// diffLayers returns (added, removed, changed, unchanged) layer slices.
// changed is a slice of [2]spec.ResolvedLayer pairs (before, after).
func diffLayers(layers1, layers2 []spec.ResolvedLayer) (
	added, removed []spec.ResolvedLayer,
	changed [][2]spec.ResolvedLayer,
	unchanged []spec.ResolvedLayer,
) {
	// Index by name — same name, compare versions and SHA256.
	byName1 := make(map[string]spec.ResolvedLayer, len(layers1))
	byName2 := make(map[string]spec.ResolvedLayer, len(layers2))
	for _, l := range layers1 {
		byName1[l.Name] = l
	}
	for _, l := range layers2 {
		byName2[l.Name] = l
	}

	// Removed: in 1 but not in 2.
	for name, l := range byName1 {
		if _, ok := byName2[name]; !ok {
			removed = append(removed, l)
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Name < removed[j].Name })

	// Added or changed.
	for name, l2 := range byName2 {
		l1, ok := byName1[name]
		if !ok {
			added = append(added, l2)
			continue
		}
		if l1.Version == l2.Version && l1.SHA256 == l2.SHA256 {
			unchanged = append(unchanged, l2)
		} else {
			changed = append(changed, [2]spec.ResolvedLayer{l1, l2})
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Name < added[j].Name })
	sort.Slice(changed, func(i, j int) bool { return changed[i][0].Name < changed[j][0].Name })
	sort.Slice(unchanged, func(i, j int) bool { return unchanged[i].Name < unchanged[j].Name })
	return
}

// diffStringMap compares two string maps, returning human-readable diff lines.
func diffStringMap(m1, m2 map[string]string) []string {
	keys := make(map[string]struct{})
	for k := range m1 {
		keys[k] = struct{}{}
	}
	for k := range m2 {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var diffs []string
	for _, k := range sorted {
		v1, ok1 := m1[k]
		v2, ok2 := m2[k]
		switch {
		case ok1 && !ok2:
			diffs = append(diffs, fmt.Sprintf("- %s=%s", k, v1))
		case !ok1 && ok2:
			diffs = append(diffs, fmt.Sprintf("+ %s=%s", k, v2))
		case v1 != v2:
			diffs = append(diffs, fmt.Sprintf("~ %s: %s → %s", k, v1, v2))
		}
	}
	return diffs
}

// diffPackages compares package sets, returning human-readable diff lines.
func diffPackages(ps1, ps2 []spec.ResolvedPackageSet) []string {
	// Flatten to manager:name → version for comparison.
	type pkgKey struct{ manager, name string }
	flat1 := make(map[pkgKey]string)
	flat2 := make(map[pkgKey]string)
	mgr1 := make(map[pkgKey]spec.PackageManager)
	mgr2 := make(map[pkgKey]spec.PackageManager)
	for _, ps := range ps1 {
		for _, p := range ps.Packages {
			k := pkgKey{string(ps.Manager), p.Name}
			flat1[k] = p.Version
			mgr1[k] = ps.Manager
		}
	}
	for _, ps := range ps2 {
		for _, p := range ps.Packages {
			k := pkgKey{string(ps.Manager), p.Name}
			flat2[k] = p.Version
			mgr2[k] = ps.Manager
		}
	}

	keys := make(map[pkgKey]struct{})
	for k := range flat1 {
		keys[k] = struct{}{}
	}
	for k := range flat2 {
		keys[k] = struct{}{}
	}
	sorted := make([]pkgKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].manager != sorted[j].manager {
			return sorted[i].manager < sorted[j].manager
		}
		return sorted[i].name < sorted[j].name
	})

	var diffs []string
	for _, k := range sorted {
		v1, ok1 := flat1[k]
		v2, ok2 := flat2[k]
		mgr := mgr1[k]
		if mgr == "" {
			mgr = mgr2[k]
		}
		switch {
		case ok1 && !ok2:
			diffs = append(diffs, fmt.Sprintf("- %s %s (%s)", k.name, v1, mgr))
		case !ok1 && ok2:
			diffs = append(diffs, fmt.Sprintf("+ %s %s (%s)", k.name, v2, mgr))
		case v1 != v2:
			diffs = append(diffs, fmt.Sprintf("~ %s %s → %s (%s)", k.name, v1, v2, mgr))
		}
	}
	return diffs
}
