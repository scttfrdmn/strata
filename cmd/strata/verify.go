package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/packages"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newVerifyCmd() *cobra.Command {
	var rekorFlag, packagesFlag bool

	cmd := &cobra.Command{
		Use:   "verify <lock.yaml>",
		Short: "Verify all layer signatures in a lockfile",
		Long: `Without --rekor, performs field-presence checks: every layer must have
non-empty Bundle and RekorEntry fields and the lockfile itself must be signed.
All failures are collected and reported together.

With --rekor, each layer's log entry is fetched from the live Rekor transparency
log and compared against that layer's Sigstore bundle: same artifact digest, same
signature, same key material. Requires network access to rekor.sigstore.dev, and
requires each layer's bundle to be readable locally — a bundle still held as an
s3:// URI is reported as a failure rather than skipped.

With --packages, each pip package's SHA256 pin is verified against PyPI.
Requires network access to pypi.org.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			lf, err := spec.ParseLockFile(args[0])
			if err != nil {
				return fmt.Errorf("verify: %w", err)
			}

			failures := collectPresenceFailures(lf)

			if rekorFlag && len(failures) == 0 {
				failures = append(failures,
					verifyRekorEntries(context.Background(), lf, &trust.RekorHTTPClient{})...)
			}

			if packagesFlag && len(lf.Packages) > 0 {
				failures = append(failures, packages.VerifyPipHashes(context.Background(), lf.Packages)...)
			}

			if len(failures) > 0 {
				fmt.Fprintf(os.Stderr, "strata verify: %d failure(s):\n", len(failures)) //nolint:errcheck
				for _, f := range failures {
					fmt.Fprintf(os.Stderr, "  - %s\n", f) //nolint:errcheck
				}
				return errors.New("") // already printed; suppress double-print in main
			}

			pkgCount := 0
			for _, ps := range lf.Packages {
				pkgCount += len(ps.Packages)
			}
			if packagesFlag && pkgCount > 0 {
				fmt.Printf("ok: %s (%d layer(s), %d package(s) verified)\n", args[0], len(lf.Layers), pkgCount)
			} else {
				fmt.Printf("ok: %s (%d layer(s) verified)\n", args[0], len(lf.Layers))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&rekorFlag, "rekor", false, "compare each layer's bundle against its entry in the live transparency log")
	cmd.Flags().BoolVar(&packagesFlag, "packages", false, "verify pip SHA256 pins against PyPI (requires network)")
	return cmd
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

// verifyRekorEntries confirms, for every layer, that the Rekor log entry named
// by RekorEntry attests that layer's bundle. Results are collected in parallel.
//
// The bundle is loaded and passed: trust.RekorClient.VerifyEntry compares the log
// entry body against it, and without one it can only establish that somebody
// logged something at that index (#59). A layer whose bundle URI cannot be read
// locally is reported as a failure naming the layer — verification that cannot be
// performed is not verification that passed.
//
// client is a parameter so a test can observe what this function passes; it used
// to construct its own RekorHTTPClient, which made the call site unobservable.
func verifyRekorEntries(ctx context.Context, lf *spec.LockFile, client trust.RekorClient) []string {
	type result struct {
		msg string
	}

	results := make(chan result, len(lf.Layers))

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
			bundle, err := loadLocalBundle(layer.Bundle)
			if err != nil {
				results <- result{fmt.Sprintf("layer %s: %v", layer.ID, err)}
				return
			}
			if err := client.VerifyEntry(ctx, idx, bundle); err != nil {
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

// loadLocalBundle reads and parses a layer's Sigstore bundle from a local path or
// a file:// URI.
//
// Registry manifests carry s3:// bundle URIs, and fetching those is the caller's
// job everywhere else in this codebase (see trust.VerifyLayer, which refuses URIs
// outright). Rather than pass a nil bundle and let the Rekor check degrade into a
// presence check, an unfetchable bundle is an error that names what is missing.
// Fetching remote bundles so that --rekor works against an s3-backed lockfile is
// tracked separately (#60).
func loadLocalBundle(uri string) (*trust.Bundle, error) {
	if uri == "" {
		return nil, errors.New("Bundle field is empty: nothing to verify the log entry against")
	}
	path := uri
	switch {
	case strings.HasPrefix(uri, "file://"):
		path = strings.TrimPrefix(uri, "file://")
	case strings.Contains(uri, "://"):
		return nil, fmt.Errorf("bundle %q must be fetched to disk before --rekor can verify against it", uri)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bundle %q: %w", path, err)
	}
	bundle, err := trust.ParseBundle(data)
	if err != nil {
		return nil, fmt.Errorf("parsing bundle %q: %w", path, err)
	}
	return bundle, nil
}
