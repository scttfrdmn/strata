// Command strata is the Strata CLI.
//
// Usage:
//
//	strata resolve <profile.yaml> [-o output.lock.yaml] [--strata-version v]
//	strata freeze  <profile.yaml> [-o output.lock.yaml]
//	strata search  [name] [--arch x86_64|arm64] [--family rhel|debian] [--formation]
//	strata verify  <lock.yaml> [--rekor]
//	strata publish <lock.yaml> [--token TOKEN] [--sandbox]
//	strata version
//
// See https://github.com/scttfrdmn/strata for documentation.
package main

import (
	"fmt"
	"os"
)

// version is the CLI version, overridden at release with -ldflags "-X main.version=1.0.0".
var version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "resolve":
		runResolve(os.Args[2:])
	case "freeze":
		runFreeze(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "publish":
		runPublish(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("strata " + version)
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "strata: unknown command %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, "Usage: strata <command> [options]\n\nCommands:\n"+ //nolint:errcheck
		"  resolve  <profile.yaml>   Resolve a profile to a lockfile\n"+
		"  freeze   <profile.yaml>   Resolve and require all layers to be SHA256-pinned\n"+
		"  search   [name]           Search the embedded layer/formation catalog\n"+
		"  verify   <lock.yaml>      Verify all layer signatures in a lockfile\n"+
		"  publish  <lock.yaml>      Publish a frozen lockfile to Zenodo (mint DOI)\n"+
		"  version                   Print strata version\n\n"+
		"Run \"strata <command> --help\" for command-specific options.\n")
}

// fatal prints an error message to stderr and exits with code 1.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "strata: "+format+"\n", args...)
	os.Exit(1)
}
