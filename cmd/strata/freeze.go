package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/scttfrdmn/strata/internal/resolver"
)

// runFreeze implements "strata freeze <profile.yaml>".
//
// It resolves the profile identically to "strata resolve", then verifies
// that the resulting lockfile is fully frozen — all layers must have a SHA256
// and the base AMI must have a SHA256. If any are missing, it reports which
// layers need to be built and pushed to the registry before freezing.
func runFreeze(args []string) {
	fs := flag.NewFlagSet("freeze", flag.ExitOnError)
	output := fs.String("o", "", "output lockfile path (default: <profile-basename>.lock.yaml)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: strata freeze <profile.yaml> [-o output.lock.yaml]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		fatal("freeze: %v", err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	profile := loadProfile(fs.Arg(0))
	reg := buildRegistryClient()
	probeClient := buildProbeClient()

	r, err := resolver.New(resolver.Config{
		Registry:      reg,
		Probe:         probeClient,
		StrataVersion: version,
	})
	if err != nil {
		fatal("freeze: %v", err)
	}

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		fatal("freeze: %v", err)
	}

	if !lf.IsFrozen() {
		var missing []string
		for _, layer := range lf.Layers {
			if layer.SHA256 == "" {
				missing = append(missing, layer.ID)
			}
		}
		fmt.Fprintf(os.Stderr,
			"strata freeze: lockfile is not frozen — layers must be built and pushed to the registry before freezing\n")
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "missing SHA256 for: %s (%d layer", joinNames(missing), len(missing))
			if len(missing) != 1 {
				fmt.Fprintf(os.Stderr, "s")
			}
			fmt.Fprintf(os.Stderr, ")\n")
		}
		os.Exit(1)
	}

	outPath := resolveOutputPath(fs.Arg(0), *output, ".lock.yaml")
	writeYAML(outPath, lf)
	fmt.Printf("frozen: %s\n", outPath)
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
