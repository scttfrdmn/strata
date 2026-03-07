package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// runVerify implements "strata verify <lock.yaml> [--rekor]".
//
// Without --rekor it performs field-presence checks: every layer must have
// non-empty Bundle and RekorEntry fields, and the lockfile itself must be
// signed (IsSigned()). All failures are collected and reported together.
//
// With --rekor each layer's RekorEntry is verified against the Rekor
// transparency log via the public Rekor API.
func runVerify(args []string) {
	fset := flag.NewFlagSet("verify", flag.ExitOnError)
	rekorFlag := fset.Bool("rekor", false, "verify each layer's Rekor entry against the live transparency log")
	fset.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: strata verify <lock.yaml> [--rekor]\n")
		fset.PrintDefaults()
	}
	if err := fset.Parse(args); err != nil {
		fatal("verify: %v", err)
	}
	if fset.NArg() != 1 {
		fset.Usage()
		os.Exit(1)
	}

	lf, err := spec.ParseLockFile(fset.Arg(0))
	if err != nil {
		fatal("verify: %v", err)
	}

	failures := collectPresenceFailures(lf)

	if *rekorFlag && len(failures) == 0 {
		failures = append(failures, verifyRekorEntries(context.Background(), lf)...)
	}

	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "strata verify: %d failure(s):\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		os.Exit(1)
	}

	fmt.Printf("ok: %s (%d layer(s) verified)\n", fset.Arg(0), len(lf.Layers))
}

// collectPresenceFailures returns a list of field-presence violation messages.
func collectPresenceFailures(lf *spec.LockFile) []string {
	var failures []string

	if !lf.IsSigned() {
		failures = append(failures, "lockfile has no RekorEntry (not signed)")
	}

	for _, layer := range lf.Layers {
		if layer.Bundle == "" {
			failures = append(failures, fmt.Sprintf("layer %s: Bundle field is empty", layer.ID))
		}
		if layer.RekorEntry == "" {
			failures = append(failures, fmt.Sprintf("layer %s: RekorEntry field is empty", layer.ID))
		}
	}
	return failures
}

// verifyRekorEntries contacts the Rekor API to confirm each layer's log entry.
// Results are collected in parallel.
func verifyRekorEntries(ctx context.Context, lf *spec.LockFile) []string {
	type result struct {
		msg string
	}

	results := make(chan result, len(lf.Layers))
	rekorClient := &trust.RekorHTTPClient{}

	var wg sync.WaitGroup
	for _, layer := range lf.Layers {
		layer := layer
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx, err := strconv.ParseInt(layer.RekorEntry, 10, 64)
			if err != nil {
				results <- result{fmt.Sprintf("layer %s: RekorEntry %q is not a valid log index: %v",
					layer.ID, layer.RekorEntry, err)}
				return
			}
			if err := rekorClient.VerifyEntry(ctx, idx, nil); err != nil {
				results <- result{fmt.Sprintf("layer %s: Rekor verification failed: %v", layer.ID, err)}
				return
			}
			results <- result{}
		}()
	}

	wg.Wait()
	close(results)

	var failures []string
	for r := range results {
		if r.msg != "" {
			failures = append(failures, r.msg)
		}
	}
	return failures
}
