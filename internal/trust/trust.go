// Package trust implements Sigstore/cosign/Rekor signing and verification.
//
// Trust is cryptographic and unconditional. Unsigned artifacts will not
// mount. There is no flag to skip verification anywhere in this package.
//
// # Trust tiers
//
//	Tier 0  Strata core layers    — signed by Strata maintainer key
//	Tier 1  Community layers      — signed by Strata CI key after recipe review
//	Tier 2  Institutional layers  — signed by institution key (bring your own)
//	Tier 3  User/local layers     — signed by user's cosign key
//
// The verification chain is: profile SHA256 → layer SHA256s + Rekor entries
// → lockfile Rekor entry. Every element is independently auditable without
// trusting Strata or its registry.
package trust

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// BundleMediaType is the media type of the sigstore bundle format used by Strata.
// Bundles in this format can be independently verified with standard cosign tooling.
const BundleMediaType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// Bundle is the attestation record stored alongside every signed artifact
// in the registry. It conforms to the sigstore bundle format so that
// any verifier with the cosign tool can independently check signatures
// without trusting Strata.
type Bundle struct {
	MediaType            string               `json:"mediaType"`
	VerificationMaterial VerificationMaterial `json:"verificationMaterial"`
	MessageSignature     MessageSignature     `json:"messageSignature"`
}

// VerificationMaterial holds the key material and Rekor transparency log entries.
type VerificationMaterial struct {
	// Certificate holds the short-lived certificate for keyless (OIDC) signatures.
	// Exactly one of Certificate or PublicKey will be set.
	Certificate *RawMaterial `json:"certificate,omitempty"`

	// PublicKey holds the raw public key for keyed signatures.
	PublicKey *RawMaterial `json:"publicKey,omitempty"`

	// TlogEntries are the Rekor transparency log entries for this bundle.
	// Strata requires at least one entry (Rekor logging is mandatory).
	TlogEntries []TlogEntry `json:"tlogEntries,omitempty"`
}

// RawMaterial holds a DER-encoded certificate or public key.
type RawMaterial struct {
	RawBytes []byte `json:"rawBytes"`
}

// TlogEntry is a single entry in the Rekor transparency log.
type TlogEntry struct {
	// LogIndex is the entry's position in the Rekor log. Used as the
	// stable reference stored in LayerManifest.RekorEntry.
	LogIndex string `json:"logIndex"`

	// LogID is the SHA256 of the Rekor instance's public key, identifying
	// which Rekor instance holds this entry.
	LogID string `json:"logID"`

	// IntegratedTime is the Unix timestamp (seconds) when Rekor accepted
	// this entry.
	IntegratedTime string `json:"integratedTime"`

	// CanonicalizedBody is the base64-encoded canonical form of the entry
	// body as stored in Rekor.
	CanonicalizedBody string `json:"canonicalizedBody"`

	// InclusionPromise is the Rekor server's signed promise that this
	// entry will be included in the log.
	InclusionPromise *InclusionPromise `json:"inclusionPromise,omitempty"`
}

// InclusionPromise is the Rekor server's signed timestamp promise.
type InclusionPromise struct {
	SignedEntryTimestamp []byte `json:"signedEntryTimestamp"`
}

// MessageSignature holds the signature over the artifact's content hash.
type MessageSignature struct {
	MessageDigest MessageDigest `json:"messageDigest"`
	// Signature is the raw signature bytes over MessageDigest.Digest.
	Signature []byte `json:"signature"`
}

// MessageDigest identifies the hash algorithm and value for the signed content.
type MessageDigest struct {
	// Algorithm is the hash algorithm, e.g. "SHA2_256".
	Algorithm string `json:"algorithm"`
	// Digest is the raw hash bytes.
	Digest []byte `json:"digest"`
}

// RekorLogIndex returns the integer log index from the first TlogEntry,
// and reports whether it was present.
func (b *Bundle) RekorLogIndex() (int64, bool) {
	if len(b.VerificationMaterial.TlogEntries) == 0 {
		return 0, false
	}
	idx, err := strconv.ParseInt(b.VerificationMaterial.TlogEntries[0].LogIndex, 10, 64)
	if err != nil {
		return 0, false
	}
	return idx, true
}

// HasRekorEntry reports whether the bundle contains at least one Rekor
// transparency log entry. Bundles without Rekor entries are not accepted
// by Strata — logging is unconditional.
func (b *Bundle) HasRekorEntry() bool {
	return len(b.VerificationMaterial.TlogEntries) > 0
}

// Marshal serializes the bundle to JSON bytes.
func (b *Bundle) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

// ParseBundle deserializes a bundle from JSON bytes.
func ParseBundle(data []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing bundle JSON: %w", err)
	}
	if b.MediaType != BundleMediaType {
		return nil, fmt.Errorf("unsupported bundle media type %q (want %q)",
			b.MediaType, BundleMediaType)
	}
	return &b, nil
}

// Signer creates cosign bundle attestations for artifacts.
// Implementations must log every signature to the Rekor transparency log —
// there is no unsigned-but-valid code path.
type Signer interface {
	// Sign signs artifactPath and returns a Bundle containing the signature,
	// key material, and Rekor transparency log entry.
	// annotations are additional key/value pairs included in the Rekor entry,
	// e.g. {"strata.layer.name": "python", "strata.layer.version": "3.11.9"}.
	Sign(ctx context.Context, artifactPath string, annotations map[string]string) (*Bundle, error)
}

// Verifier checks cosign bundle attestations for artifacts.
// Implementations must verify both the cryptographic signature and the Rekor
// entry — there is no partial verification. An error means the artifact is
// not trusted and must not be used.
type Verifier interface {
	// Verify checks that artifactPath matches bundle's signature and that
	// the bundle's Rekor entry is valid and current.
	// Returns nil iff both checks pass. Any failure returns a non-nil error.
	Verify(ctx context.Context, artifactPath string, bundle *Bundle) error
}

// RekorClient provides direct access to the Rekor transparency log.
type RekorClient interface {
	// Log submits bundle to the Rekor transparency log and returns the
	// log index. The returned index is stored in LayerManifest.RekorEntry.
	Log(ctx context.Context, bundle *Bundle) (logIndex int64, err error)

	// VerifyEntry confirms that a bundle is present and unmodified in the
	// Rekor log at logIndex. Returns nil iff the entry is valid.
	VerifyEntry(ctx context.Context, logIndex int64, bundle *Bundle) error
}
