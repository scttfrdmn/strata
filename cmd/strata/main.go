// Command strata is the Strata CLI.
//
// Usage:
//
//	strata resolve <profile.yaml>    resolve a profile to a lockfile
//	strata freeze  <profile.yaml>    produce a fully pinned lockfile
//	strata publish <lock.yaml>       publish lockfile to Zenodo, mint DOI
//	strata search  <name>            search the registry for a layer or formation
//	strata verify  <lock.yaml>       verify all layer signatures against Rekor
//
// See https://github.com/scttfrdmn/strata for documentation.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "strata: not yet implemented")
	os.Exit(1)
}
