// Command strata is the Strata CLI.
//
// Usage:
//
//	strata resolve        <profile.yaml> [-o output.lock.yaml] [--strata-version v]
//	strata freeze         <profile.yaml> [-o output.lock.yaml]
//	strata run            --lockfile <lock.yaml> -- <command> [args...]
//	strata export         --lockfile <lock.yaml> --format oci --output <file.tar>
//	strata search         [name] [--arch x86_64|arm64] [--family rhel|debian] [--formation]
//	strata update         <profile.yaml> [-o output.lock.yaml]
//	strata verify         <lock.yaml> [--rekor] [--packages]
//	strata publish        <lock.yaml> [--token TOKEN] [--sandbox]
//	strata probe          <os> <arch> [--registry s3://...]
//	strata build          <recipe-dir> [options]
//	strata build-catalog  <recipes-dir> [options]
//	strata index          --registry s3://...
//
// See https://github.com/scttfrdmn/strata for documentation.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is the CLI version, overridden at release with -ldflags "-X main.version=1.0.0".
var version = "0.0.0-dev"

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		if err.Error() != "" {
			fmt.Fprintf(os.Stderr, "strata: %v\n", err)
		}
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "strata",
		Short:         "Composable, reproducible, cryptographically attested compute environments",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newResolveCmd(),
		newFreezeCmd(),
		newRunCmd(),
		newExportCmd(),
		newSearchCmd(),
		newVerifyCmd(),
		newPublishCmd(),
		newProbeCmd(),
		newBuildCmd(),
		newBuildCatalogCmd(),
		newIndexCmd(),
		newRemoveCmd(),
		newCachePruneCmd(),
		newFreezeLayerCmd(),
		newSnapshotAMICmd(),
		newScanCmd(),
		newCaptureCmd(),
		newStratifyCmd(),
		newFoldCmd(),
		newDiffCmd(),
		newUpdateCmd(),
	)

	return root
}
