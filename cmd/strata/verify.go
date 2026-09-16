// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/packages"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newVerifyCmd() *cobra.Command {
	var rekorFlag, packagesFlag bool
	var maxAgeFlag string

	cmd := &cobra.Command{
		Use:   "verify <lock.yaml>",
		Short: "Check layer attestation bundles in a lockfile",
		Long: `Without --rekor, checks each layer's attestation bundle without contacting
the network: the lockfile must be signed, every layer must name a Bundle and a
RekorEntry, and each bundle must parse as a Sigstore bundle carrying a Rekor
entry. This catches a bundle whose contents are not a bundle at all — a
presence check alone would accept prose. It is NOT signature verification: the
layer content is not present to hash and no trust root is consulted. A bundle
still held as an s3:// URI is reported as needing a fetch, not skipped.
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

			// Disclose that packages: entries sit outside the layer attestation
			// chain (#139), distinct from a layer's missing signature — a fully
			// layer-verified environment can still contain unattested code.
			if note := packagesUnattestedNote(lf); note != "" {
				fmt.Fprintln(os.Stderr, note) //nolint:errcheck
			}

			// Surface how far behind this lockfile is (#224, T8). Always shown —
			// verify's failure mode is a green, and a stale but validly-signed
			// lockfile passing silently is exactly that green. --max-age turns the
			// staleness into a failure so the "ok" line is honest about age.
			maxAge, err := spec.ParseMaxAge(maxAgeFlag)
			if err != nil {
				return err
			}
			if note := lf.FreshnessNote(time.Now()); note != "" {
				fmt.Fprintln(os.Stderr, note) //nolint:errcheck
			}

			failures := collectPresenceFailures(lf)
			failures = append(failures, freshnessFailures(lf, maxAge, time.Now())...)
			failures = append(failures, collectBundleFailures(lf)...)

			// Verify the lockfile's own signature — the whole set — against the
			// embedded public key, when the lockfile carries one. This is the real
			// VerifyLockFile #60 found missing, and the set attestation that makes a
			// mix-and-match of individually-valid layers detectable (#101). The
			// bundle carries its own Rekor inclusion proof, so this needs no AWS.
			lockfileSigned := lf.Bundle != ""
			failures = append(failures, lockfileSignatureFailures(lf)...)

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
			// The default path does not verify — it confirms each bundle is
			// well-formed and carries a Rekor entry. Only --rekor verifies the
			// bundle against the live transparency log, so only --rekor may say
			// "verified" of a layer (#60).
			pkgSuffix := ""
			if packagesFlag && pkgCount > 0 {
				pkgSuffix = fmt.Sprintf(", %d package(s) verified against PyPI", pkgCount)
			}
			// The lockfile-signature clause is the set attestation: a verified
			// signature means this exact layer set was signed, so a mix-and-match is
			// caught here even when every individual layer verifies.
			sigSuffix := "; lockfile unsigned (no set signature — run 'strata sign')"
			if lockfileSigned {
				sigSuffix = "; lockfile signature verified"
			}
			if rekorFlag {
				fmt.Printf("ok: %s (%d layer(s) verified against the transparency log%s%s)\n", args[0], len(lf.Layers), pkgSuffix, sigSuffix)
			} else {
				fmt.Printf("ok: %s (%d layer(s): bundle well-formed, attestation present%s — layers not verified against the transparency log; run 'strata verify --rekor'%s)\n", args[0], len(lf.Layers), pkgSuffix, sigSuffix)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&rekorFlag, "rekor", false, "compare each layer's bundle against its entry in the live transparency log")
	cmd.Flags().BoolVar(&packagesFlag, "packages", false, "verify pip SHA256 pins against PyPI (requires network)")
	cmd.Flags().StringVar(&maxAgeFlag, "max-age", "", "fail if the lockfile was resolved longer ago than this (e.g. 30d, 2w, 720h); default: report age only")
	return cmd
}

// freshnessFailures returns a one-element failure when a max-age bound is set
// and the lockfile is older than it, else nil (#224, T8). It is a separate
// helper so verify's honest-green contract — a stale lockfile must not pass
// silently under --max-age — is testable without executing the command. With no
// bound (maxAge == 0) it returns nil: age is only surfaced, never a failure.
func freshnessFailures(lf *spec.LockFile, maxAge time.Duration, now time.Time) []string {
	if err := lf.CheckFreshness(maxAge, now); err != nil {
		return []string{fmt.Sprintf("freshness: %v", err)}
	}
	return nil
}

// lockfileSignatureFailures verifies the lockfile's own signature — the whole
// layer set — and returns any failure, or nil when it verifies or the lockfile
// carries no signature. Verifying the set is what catches a mix-and-match of
// individually-valid layers (#101); the real VerifyLockFile #60 found missing.
func lockfileSignatureFailures(lf *spec.LockFile) []string {
	if lf.Bundle == "" {
		return nil
	}
	if err := verifyLockfileSignature(context.Background(), lf); err != nil {
		return []string{fmt.Sprintf("lockfile signature: %v", err)}
	}
	return nil
}

// verifyLockfileSignature is the seam: production verifies against the public key
// embedded in the binary (no AWS — the bundle carries its Rekor proof); a test
// injects a fake verifier.
var verifyLockfileSignature = func(ctx context.Context, lf *spec.LockFile) error {
	v, cleanup, err := trust.EmbeddedKeyVerifier()
	if err != nil {
		return err
	}
	defer cleanup()
	return trust.VerifyLockFile(ctx, lf, v)
}

// packagesUnattestedNote reports that a lockfile's package entries are outside
// the layer attestation chain, or "" when there are none. This is the disclosure
// #139 asks strata verify to make: packages: entries are fetched at boot from
// PyPI/conda-forge/CRAN by strata-agent, with no bundle and no Rekor entry, so an
// environment whose layers all verify can still contain unattested code. It is a
// note, not a failure — the layers are genuinely verified; the packages are a
// separate, disclosed category.
func packagesUnattestedNote(lf *spec.LockFile) string {
	n := 0
	for _, ps := range lf.Packages {
		n += len(ps.Packages)
	}
	if n == 0 {
		return ""
	}
	noun := "entries"
	if n == 1 {
		noun = "entry"
	}
	return fmt.Sprintf("note: %d package %s (pip/conda/cran) are installed by strata-agent at boot from "+
		"PyPI/conda-forge/CRAN — they are NOT part of the layer attestation chain (no bundle, no Rekor entry), "+
		"so this environment includes unattested content; run 'strata verify --packages' to compare recorded "+
		"pip digests against PyPI", n, noun)
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

// collectBundleFailures parses each layer's bundle and confirms it is a
// well-formed Sigstore bundle carrying a Rekor entry.
//
// This is what turns "verified" from a lie into a check without a trust root or
// network (#60): a presence test accepts a Bundle field whose file contents are
// prose, and printed "verified" over it. Parsing rejects that. It is still short
// of signature verification — the layer squashfs is not present to hash and no
// key/identity is consulted (that needs a fetch and #62), and --rekor is what
// checks the bundle against the live log. A bundle held as an unfetchable URI
// (s3://) is reported as needing a fetch rather than skipped, via loadLocalBundle.
//
// Layers with an empty Bundle are left to collectPresenceFailures, which names
// that field; parsing "" here would only duplicate the report.
func collectBundleFailures(lf *spec.LockFile) []string {
	var failures []string
	for _, layer := range lf.Layers {
		if layer.Bundle == "" {
			continue
		}
		bundle, err := loadLocalBundle(layer.Bundle)
		if err != nil {
			failures = append(failures, fmt.Sprintf("layer %s: %v", layer.ID, err))
			continue
		}
		if !bundle.HasRekorEntry() {
			failures = append(failures, fmt.Sprintf("layer %s: bundle is not a signed Sigstore bundle (no Rekor entry)", layer.ID))
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
// Fetching remote bundles so that s3-backed lockfiles can be verified in place is
// tracked separately (#60).
//
// Callers: strata verify --rekor, and strata run's pre-mount check (#55). The
// messages are worded for both — they used to name --rekor, which would have been
// a lie in run's output. Nothing else about this function changed when run started
// calling it: it neither fetches nor mutates, and every failure it can report is
// returned rather than logged, so the second caller inherits no behaviour that the
// first one's tests were not already asserting.
func loadLocalBundle(uri string) (*trust.Bundle, error) {
	if uri == "" {
		return nil, errors.New("empty Bundle field: nothing to verify the signature against")
	}
	path := uri
	switch {
	case strings.HasPrefix(uri, "file://"):
		path = strings.TrimPrefix(uri, "file://")
	case strings.Contains(uri, "://"):
		return nil, fmt.Errorf("bundle %q must be fetched to disk before it can be verified", uri)
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
