package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/scttfrdmn/strata/internal/zenodo"
	"github.com/scttfrdmn/strata/spec"
)

// runPublish implements "strata publish <lock.yaml> [--token TOKEN] [--sandbox]".
//
// It publishes a frozen lockfile to Zenodo via the Deposit API and prints
// the minted DOI. The lockfile must be fully frozen (all layers have SHA256).
// The Zenodo token must be provided via --token or the ZENODO_TOKEN env var.
func runPublish(args []string) {
	fset := flag.NewFlagSet("publish", flag.ExitOnError)
	token := fset.String("token", "", "Zenodo personal access token (overrides ZENODO_TOKEN env var)")
	sandbox := fset.Bool("sandbox", false, "use sandbox.zenodo.org instead of production")
	fset.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: strata publish <lock.yaml> [--token TOKEN] [--sandbox]\n")
		fset.PrintDefaults()
	}
	if err := fset.Parse(args); err != nil {
		fatal("publish: %v", err)
	}
	if fset.NArg() != 1 {
		fset.Usage()
		os.Exit(1)
	}

	tok := resolveToken(*token)
	lf := parseFrozenLockFile(fset.Arg(0))

	client := &zenodo.Client{Token: tok}
	if *sandbox {
		client.BaseURL = "https://sandbox.zenodo.org"
	}

	result, err := client.Deposit(context.Background(), lf)
	if err != nil {
		fatal("publish: %v", err)
	}

	fmt.Printf("published: doi:%s\n", result.DOI)
	fmt.Printf("record:    %s\n", result.RecordURL)
}

// resolveToken returns the token from the flag or ZENODO_TOKEN env var.
// It calls fatal if neither is set.
func resolveToken(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if t := os.Getenv("ZENODO_TOKEN"); t != "" {
		return t
	}
	fatal("publish: Zenodo token required — set ZENODO_TOKEN or use --token")
	return "" // unreachable
}

// parseFrozenLockFile reads a lockfile and verifies it is frozen.
// Calls fatal if the lockfile is not frozen.
func parseFrozenLockFile(path string) *spec.LockFile {
	lf, err := spec.ParseLockFile(path)
	if err != nil {
		fatal("publish: %v", err)
	}
	if !lf.IsFrozen() {
		fatal("publish: lockfile is not frozen — run \"strata freeze\" first to pin all layer SHA256s")
	}
	return lf
}
