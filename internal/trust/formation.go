// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"context"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// formationSigningPayload is the canonical byte sequence a formation signature
// covers: the whole formation with its signature/provenance fields (Bundle,
// RekorEntry, SignedBy) zeroed, so the signature commits to every other field —
// the name and version, the ordered layer refs, and the provided capabilities —
// but not to the attestation material set at signing time. Signing and verifying
// compute it identically, so a signature is valid only for the exact formation
// content it was produced from.
//
// This is the formation analog of lockfileSigningPayload (#101): the formation
// is signed "as a unit" (its own godoc's claim), so a formation whose layer set
// or provides no maintainer attested has no valid signature — closing the
// pending-initial-build placeholders #237 found in every shipped formation.
func formationSigningPayload(f *spec.Formation) ([]byte, error) {
	clone := *f
	clone.Bundle = ""
	clone.RekorEntry = ""
	clone.SignedBy = ""
	return yaml.Marshal(&clone)
}

// SignFormation signs the formation's canonical content with signer and records
// the resulting Sigstore bundle on the formation — Bundle holds the bundle JSON,
// RekorEntry the transparency-log index. In production signer is
// CosignSigner{KeyRef: <KMS URI>}, which logs the signature to Rekor so the
// formation carries the same transparency as the layers and lockfiles. The
// signature is required to carry a transparency-log entry, mirroring
// SignLockFile: a formation signature that is not logged is refused rather than
// silently accepted.
//
// It does not set SignedBy — that is the human-readable key ref, provenance the
// caller records (as the build does for layer manifests); it is excluded from
// the signed payload for the same reason Bundle/RekorEntry are.
func SignFormation(ctx context.Context, f *spec.Formation, signer Signer) error {
	payload, err := formationSigningPayload(f)
	if err != nil {
		return fmt.Errorf("trust: formation signing payload: %w", err)
	}
	path, cleanup, err := writePayloadTemp(payload)
	if err != nil {
		return fmt.Errorf("trust: writing formation payload: %w", err)
	}
	defer cleanup()

	bundle, err := signer.Sign(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("trust: signing formation: %w", err)
	}
	data, err := bundle.Marshal()
	if err != nil {
		return fmt.Errorf("trust: marshaling formation bundle: %w", err)
	}
	idx, ok := bundle.RekorLogIndex()
	if !ok {
		return fmt.Errorf("trust: signer returned a bundle with no transparency-log entry — a formation signature must be logged to Rekor")
	}
	f.Bundle = string(data)
	f.RekorEntry = strconv.FormatInt(idx, 10)
	return nil
}

// VerifyFormation verifies the formation's own signature with verifier — the
// agent's embedded key in production. A tampered field, a swapped or reordered
// layer ref, or a forged set fails here because the signature covers the whole
// formation, and the verifier also rejects a bundle with no transparency-log
// entry. It is the formation analog of VerifyLockFile.
func VerifyFormation(ctx context.Context, f *spec.Formation, verifier Verifier) error {
	if f.Bundle == "" {
		return fmt.Errorf("trust: formation is not signed — no bundle to verify")
	}
	bundle, err := ParseBundle([]byte(f.Bundle))
	if err != nil {
		return fmt.Errorf("trust: parsing formation bundle: %w", err)
	}
	payload, err := formationSigningPayload(f)
	if err != nil {
		return fmt.Errorf("trust: formation signing payload: %w", err)
	}
	path, cleanup, err := writePayloadTemp(payload)
	if err != nil {
		return fmt.Errorf("trust: writing formation payload: %w", err)
	}
	defer cleanup()

	if err := verifier.Verify(ctx, path, bundle); err != nil {
		return fmt.Errorf("trust: formation signature verification failed: %w", err)
	}
	// Bind the claimed transparency-log pointer to the bundle, as VerifyLockFile
	// does: RekorEntry is excluded from the signed payload (it cannot be signed
	// before it exists), yet it is published as the formation's attestation
	// pointer, so a valid set signature must not accompany a falsified one.
	idx, ok := bundle.RekorLogIndex()
	if !ok {
		return fmt.Errorf("trust: formation bundle carries no transparency-log entry")
	}
	if f.RekorEntry != strconv.FormatInt(idx, 10) {
		return fmt.Errorf("trust: formation rekor_entry %q does not match the signed bundle's log index %d", f.RekorEntry, idx)
	}
	return nil
}
