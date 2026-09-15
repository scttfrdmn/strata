// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// lockfileSigningPayload is the canonical byte sequence a lockfile signature
// covers: the whole lockfile with its own signature fields (Bundle, RekorEntry)
// zeroed, so the signature commits to every other field — the layer set and its
// mount order, the packages, the base, resolved_at (which a freshness bound
// relies on being signed), the verification policy — but not to itself. Signing
// and verifying compute it identically, so a signature is valid only for the
// exact lockfile content it was produced from.
//
// It signs the whole set, not the per-layer digests, which is what makes a
// mix-and-match of individually-valid, individually-signed layers detectable
// (#101): a combination no maintainer ever attested has no valid lockfile
// signature, however well each layer verifies on its own.
func lockfileSigningPayload(lf *spec.LockFile) ([]byte, error) {
	clone := *lf
	clone.Bundle = ""
	clone.RekorEntry = ""
	return yaml.Marshal(&clone)
}

// writePayloadTemp materialises payload to a temp file for cosign, which signs
// and verifies a file path. The returned cleanup removes it.
func writePayloadTemp(payload []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "strata-lockfile-*.yaml")
	if err != nil {
		return "", func() {}, err
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

// SignLockFile signs the lockfile's canonical content with signer and records the
// resulting Sigstore bundle on the lockfile — Bundle holds the bundle JSON,
// RekorEntry the transparency-log index. In production signer is
// CosignSigner{KeyRef: <KMS URI>}, which logs the signature to Rekor so the
// lockfile has the same transparency as its layers; strata already needs the
// network to pull layers from S3, so the Rekor round-trip costs nothing extra.
func SignLockFile(ctx context.Context, lf *spec.LockFile, signer Signer) error {
	payload, err := lockfileSigningPayload(lf)
	if err != nil {
		return fmt.Errorf("trust: lockfile signing payload: %w", err)
	}
	path, cleanup, err := writePayloadTemp(payload)
	if err != nil {
		return fmt.Errorf("trust: writing lockfile payload: %w", err)
	}
	defer cleanup()

	bundle, err := signer.Sign(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("trust: signing lockfile: %w", err)
	}
	data, err := bundle.Marshal()
	if err != nil {
		return fmt.Errorf("trust: marshaling lockfile bundle: %w", err)
	}
	// Require a transparency-log entry rather than letting a stale RekorEntry
	// survive a signer that returns none: the chosen anchor is key + Rekor, and
	// RekorEntry is the artifact's public provenance pointer (exported as
	// lockfile_rekor_entry). Derive it from the bundle so it cannot drift.
	idx, ok := bundle.RekorLogIndex()
	if !ok {
		return fmt.Errorf("trust: signer returned a bundle with no transparency-log entry — a lockfile signature must be logged to Rekor")
	}
	lf.Bundle = string(data)
	lf.RekorEntry = strconv.FormatInt(idx, 10)
	return nil
}

// VerifyLockFile verifies the lockfile's own signature with verifier — the
// agent's embedded key in production (CosignVerifier{KeyRef: <embedded>}). It is
// the check #60 found missing: a real lockfile verification that verify, publish,
// and the agent can all call, so they cannot drift on what "signed" means. A
// tampered field, a swapped or reordered layer, or a forged set fails here
// because the signature covers the whole lockfile, and the verifier also rejects
// a bundle with no transparency-log entry.
func VerifyLockFile(ctx context.Context, lf *spec.LockFile, verifier Verifier) error {
	if lf.Bundle == "" {
		return fmt.Errorf("trust: lockfile is not signed — no bundle to verify")
	}
	bundle, err := ParseBundle([]byte(lf.Bundle))
	if err != nil {
		return fmt.Errorf("trust: parsing lockfile bundle: %w", err)
	}
	payload, err := lockfileSigningPayload(lf)
	if err != nil {
		return fmt.Errorf("trust: lockfile signing payload: %w", err)
	}
	path, cleanup, err := writePayloadTemp(payload)
	if err != nil {
		return fmt.Errorf("trust: writing lockfile payload: %w", err)
	}
	defer cleanup()

	if err := verifier.Verify(ctx, path, bundle); err != nil {
		return fmt.Errorf("trust: lockfile signature verification failed: %w", err)
	}
	// Bind the claimed transparency-log pointer to the bundle. RekorEntry is
	// excluded from the signed payload — it cannot be signed before it exists — so
	// a valid set signature does not by itself vouch for lf.RekorEntry, yet
	// ProvenanceRecord publishes it as lockfile_rekor_entry (a citation a paper or
	// DOI record may treat as authoritative). Reject a value that does not match
	// the signed bundle's own log index, so "signature verified" cannot accompany
	// a falsified provenance pointer.
	idx, ok := bundle.RekorLogIndex()
	if !ok {
		return fmt.Errorf("trust: lockfile bundle carries no transparency-log entry")
	}
	if lf.RekorEntry != strconv.FormatInt(idx, 10) {
		return fmt.Errorf("trust: lockfile rekor_entry %q does not match the signed bundle's log index %d", lf.RekorEntry, idx)
	}
	return nil
}
