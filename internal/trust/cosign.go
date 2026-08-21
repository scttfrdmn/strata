package trust

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CosignToolVersion returns the version string of the cosign binary on PATH
// (e.g. "v3.0.5"). Returns "unknown" if cosign is not found or the version
// cannot be parsed. The result is suitable for recording in LayerManifest.
func CosignToolVersion(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "cosign", "version").Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "GitVersion:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

// CosignSigner implements Signer using the cosign CLI.
// It requires cosign to be installed and available on PATH.
// The key is a cosign key file path or a KMS URI.
type CosignSigner struct {
	// KeyRef is the signing key reference: a file path (e.g. "cosign.key"),
	// a KMS URI (e.g. "gcpkms://..."), or empty for keyless (OIDC) signing.
	KeyRef string
}

// Sign invokes cosign sign-blob to sign artifactPath and returns the bundle.
func (s *CosignSigner) Sign(ctx context.Context, artifactPath string, annotations map[string]string) (*Bundle, error) {
	bundleFile, err := os.CreateTemp("", "strata-bundle-*.json")
	if err != nil {
		return nil, fmt.Errorf("cosign sign: creating temp bundle file: %w", err)
	}
	bundlePath := bundleFile.Name()
	bundleFile.Close()          //nolint:errcheck
	defer os.Remove(bundlePath) //nolint:errcheck

	// cosign v3 dropped --annotations from sign-blob; annotations are recorded
	// in manifest.yaml instead. The signing key, SHA256, and Rekor log entry
	// provide all required provenance without embedded metadata.
	_ = annotations

	args := []string{
		"sign-blob",
		"--bundle", bundlePath,
		"--yes", // skip interactive confirmation
	}
	if s.KeyRef != "" {
		args = append(args, "--key", s.KeyRef)
	}
	args = append(args, artifactPath)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cosign sign-blob: %w\nstderr: %s", err, stderr.String())
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("cosign sign-blob: reading bundle: %w", err)
	}

	return ParseBundle(data)
}

// CosignVerifier implements Verifier using the cosign CLI.
type CosignVerifier struct {
	// KeyRef is the verification key reference: a file path, a KMS URI,
	// or empty for keyless (OIDC) verification.
	KeyRef string

	// CertIdentity is required for keyless verification — the expected
	// certificate subject (OIDC identity of the signer).
	CertIdentity string

	// CertOIDCIssuer is required for keyless verification — the expected
	// OIDC issuer URL.
	CertOIDCIssuer string
}

// Verify invokes cosign verify-blob to check the bundle against artifactPath.
func (v *CosignVerifier) Verify(ctx context.Context, artifactPath string, bundle *Bundle) error {
	if !bundle.HasRekorEntry() {
		return fmt.Errorf("cosign verify: bundle has no Rekor entry: unsigned artifacts will not mount")
	}

	data, err := bundle.Marshal()
	if err != nil {
		return fmt.Errorf("cosign verify: marshaling bundle: %w", err)
	}

	bundleFile, err := os.CreateTemp("", "strata-bundle-*.json")
	if err != nil {
		return fmt.Errorf("cosign verify: creating temp bundle file: %w", err)
	}
	bundlePath := bundleFile.Name()
	if _, err := bundleFile.Write(data); err != nil {
		bundleFile.Close()    //nolint:errcheck
		os.Remove(bundlePath) //nolint:errcheck
		return fmt.Errorf("cosign verify: writing bundle: %w", err)
	}
	if err := bundleFile.Close(); err != nil {
		os.Remove(bundlePath) //nolint:errcheck
		return fmt.Errorf("cosign verify: closing bundle file: %w", err)
	}
	defer os.Remove(bundlePath) //nolint:errcheck

	args := []string{"verify-blob", "--bundle", bundlePath}
	switch {
	case v.KeyRef != "":
		args = append(args, "--key", v.KeyRef)
	case v.CertIdentity != "" && v.CertOIDCIssuer != "":
		args = append(args, "--certificate-identity", v.CertIdentity,
			"--certificate-oidc-issuer", v.CertOIDCIssuer)
	default:
		return fmt.Errorf("cosign verify: must provide either KeyRef or CertIdentity+CertOIDCIssuer")
	}
	args = append(args, artifactPath)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign verify-blob failed: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

// RekorHTTPClient implements RekorClient via the Rekor REST API.
// It uses the public Rekor instance at https://rekor.sigstore.dev by default.
type RekorHTTPClient struct {
	// BaseURL is the Rekor server base URL. Defaults to https://rekor.sigstore.dev.
	BaseURL string
}

// rekorBaseURL returns the effective base URL.
func (c *RekorHTTPClient) rekorBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://rekor.sigstore.dev"
}

// hashedRekorBody is the minimal structure for a hashedrekord Rekor entry.
type hashedRekorBody struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		Data struct {
			Hash struct {
				Algorithm string `json:"algorithm"`
				Value     string `json:"value"`
			} `json:"hash"`
		} `json:"data"`
		Signature struct {
			Content   []byte `json:"content"`
			PublicKey struct {
				Content []byte `json:"content"`
			} `json:"publicKey"`
		} `json:"signature"`
	} `json:"spec"`
}

// Log submits a transparency log entry to Rekor for the given bundle.
// The bundle must contain a MessageSignature and VerificationMaterial.
func (c *RekorHTTPClient) Log(ctx context.Context, bundle *Bundle) (int64, error) {
	// Build the hashedrekord entry body.
	body := hashedRekorBody{
		APIVersion: "0.0.1",
		Kind:       "hashedrekord",
	}
	body.Spec.Data.Hash.Algorithm = "sha256"
	body.Spec.Data.Hash.Value = fmt.Sprintf("%x", bundle.MessageSignature.MessageDigest.Digest)
	body.Spec.Signature.Content = bundle.MessageSignature.Signature

	switch {
	case bundle.VerificationMaterial.Certificate != nil:
		body.Spec.Signature.PublicKey.Content = bundle.VerificationMaterial.Certificate.RawBytes
	case bundle.VerificationMaterial.PublicKey != nil:
		body.Spec.Signature.PublicKey.Content = bundle.VerificationMaterial.PublicKey.RawBytes
	default:
		return 0, fmt.Errorf("rekor log: bundle has neither certificate nor public key")
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("rekor log: marshaling entry: %w", err)
	}

	payload, err := json.Marshal(map[string]json.RawMessage{"proposedEntry": bodyJSON})
	if err != nil {
		return 0, fmt.Errorf("rekor log: marshaling request: %w", err)
	}

	resp, err := postJSON(ctx, c.rekorBaseURL()+"/api/v1/log/entries", payload)
	if err != nil {
		return 0, fmt.Errorf("rekor log: POST failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("rekor log: decoding response: %w", err)
	}

	// Response is a map of logID → entry; extract the logIndex from the first entry.
	for _, raw := range result {
		var entry struct {
			LogIndex int64 `json:"logIndex"`
		}
		if err := json.Unmarshal(raw, &entry); err == nil {
			return entry.LogIndex, nil
		}
	}
	return 0, fmt.Errorf("rekor log: could not extract logIndex from response")
}

// Sentinel errors returned by RekorHTTPClient.VerifyEntry. There is one per
// distinct reason an entry fails to attest a bundle, because "the call returned
// an error" is not a measurement: a rejection that fires from the wrong check
// looks identical in the output. Callers and tests match with errors.Is.
var (
	// ErrNoBundle means VerifyEntry was called without the bundle whose
	// contents the log entry is supposed to attest. Existence of an entry at a
	// log index says only that somebody logged something there.
	ErrNoBundle = errors.New("rekor verify: no bundle supplied")

	// ErrEntryNotFound means the log holds no entry at the requested index.
	ErrEntryNotFound = errors.New("rekor verify: no entry at log index")

	// ErrLogIndexMismatch means the bundle names a different log index than the
	// one being checked, so the two cannot be talking about the same entry.
	ErrLogIndexMismatch = errors.New("rekor verify: bundle claims a different log index")

	// ErrMalformedEntry means the entry body could not be decoded.
	ErrMalformedEntry = errors.New("rekor verify: entry body is not decodable")

	// ErrUnsupportedEntryKind means the entry is not a hashedrekord, which is
	// the only kind this client knows how to compare against a bundle.
	ErrUnsupportedEntryKind = errors.New("rekor verify: unsupported entry kind")

	// ErrIncompleteBundle means the bundle carries no digest or no signature,
	// so there is nothing to compare the entry against. Comparing empty to
	// empty would succeed, which is the failure mode this check exists to stop.
	ErrIncompleteBundle = errors.New("rekor verify: bundle has no digest or signature to compare")

	// ErrDigestAlgorithm means the entry or the bundle names a hash algorithm
	// other than SHA-256.
	ErrDigestAlgorithm = errors.New("rekor verify: unsupported digest algorithm")

	// ErrDigestMismatch means the entry attests a different artifact than the
	// bundle does. This is the case where a real, valid, findable log entry
	// belongs to somebody else's artifact.
	ErrDigestMismatch = errors.New("rekor verify: entry attests a different artifact digest")

	// ErrSignatureMismatch means the entry and the bundle agree on the artifact
	// but carry different signatures over it.
	ErrSignatureMismatch = errors.New("rekor verify: entry carries a different signature")

	// ErrPublicKeyMismatch means the entry and the bundle carry different key
	// material. Checked only when both sides supply raw key bytes.
	ErrPublicKeyMismatch = errors.New("rekor verify: entry carries different key material")
)

// VerifyEntry confirms that the Rekor log entry at logIndex attests bundle:
// that an entry exists there, and that its body names the same artifact digest,
// the same signature, and (where both sides carry raw key bytes) the same key as
// the bundle presented alongside it.
//
// bundle is required. Existence alone establishes only that something was logged
// at that index — not that it was this artifact, nor that it was signed by the
// expected key. An index without a bundle is unverifiable and returns
// ErrNoBundle rather than success (#59).
//
// The comparison covers the hashedrekord fields this repository itself writes in
// Log: spec.data.hash (algorithm and hex value) and spec.signature (content and
// public key). It does not check the log's own signed inclusion promise or the
// signer's identity — a bundle whose signature is cryptographically invalid but
// consistently recorded in the log still passes this function. Full signature
// verification is CosignVerifier.Verify's job; this function's job is to tie the
// log entry to the bundle.
func (c *RekorHTTPClient) VerifyEntry(ctx context.Context, logIndex int64, bundle *Bundle) error {
	if bundle == nil {
		return fmt.Errorf("%w: log index %d proves only that something was logged there", ErrNoBundle, logIndex)
	}
	// A bundle that names a different index is not evidence about this one,
	// whatever the log returns.
	if claimed, ok := bundle.RekorLogIndex(); ok && claimed != logIndex {
		return fmt.Errorf("%w: bundle names %d, caller asked about %d", ErrLogIndexMismatch, claimed, logIndex)
	}

	url := fmt.Sprintf("%s/api/v1/log/entries?logIndex=%d", c.rekorBaseURL(), logIndex)
	resp, err := getJSON(ctx, url)
	if err != nil {
		return fmt.Errorf("rekor verify: GET failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("rekor verify: decoding response: %w", err)
	}

	for _, raw := range result {
		var entry struct {
			LogIndex int64  `json:"logIndex"`
			Body     string `json:"body"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		if entry.LogIndex == logIndex {
			return matchEntryBody(entry.Body, bundle, logIndex)
		}
	}
	return fmt.Errorf("%w: %d", ErrEntryNotFound, logIndex)
}

// matchEntryBody compares a base64-encoded hashedrekord entry body against the
// bundle it is supposed to attest. Every return path names the specific field
// that disagreed, so that a caller can tell which check rejected the entry.
func matchEntryBody(bodyB64 string, bundle *Bundle, logIndex int64) error {
	rawBody, err := base64.StdEncoding.DecodeString(bodyB64)
	if err != nil {
		return fmt.Errorf("%w: entry %d body is not base64: %v", ErrMalformedEntry, logIndex, err)
	}
	var body hashedRekorBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return fmt.Errorf("%w: entry %d body is not JSON: %v", ErrMalformedEntry, logIndex, err)
	}
	if body.Kind != "hashedrekord" {
		return fmt.Errorf("%w: entry %d is kind %q; this client compares hashedrekord entries only",
			ErrUnsupportedEntryKind, logIndex, body.Kind)
	}

	// An empty digest or signature on the bundle side would make every
	// comparison below vacuous, so it is rejected before any of them run.
	if len(bundle.MessageSignature.MessageDigest.Digest) == 0 || len(bundle.MessageSignature.Signature) == 0 {
		return fmt.Errorf("%w: digest=%d bytes signature=%d bytes",
			ErrIncompleteBundle,
			len(bundle.MessageSignature.MessageDigest.Digest),
			len(bundle.MessageSignature.Signature))
	}

	if !isSHA256(body.Spec.Data.Hash.Algorithm) {
		return fmt.Errorf("%w: entry %d names %q", ErrDigestAlgorithm, logIndex, body.Spec.Data.Hash.Algorithm)
	}
	if alg := bundle.MessageSignature.MessageDigest.Algorithm; alg != "" && !isSHA256(alg) {
		return fmt.Errorf("%w: bundle names %q", ErrDigestAlgorithm, alg)
	}

	// The Rekor body carries the digest as a hex string; the bundle carries raw
	// bytes. Log writes the same bridge (fmt.Sprintf("%x", …)).
	wantDigest := hex.EncodeToString(bundle.MessageSignature.MessageDigest.Digest)
	if gotDigest := strings.ToLower(body.Spec.Data.Hash.Value); gotDigest != wantDigest {
		return fmt.Errorf("%w: entry %d attests sha256:%s, bundle attests sha256:%s",
			ErrDigestMismatch, logIndex, gotDigest, wantDigest)
	}

	if !bytes.Equal(body.Spec.Signature.Content, bundle.MessageSignature.Signature) {
		return fmt.Errorf("%w: entry %d signature is %d bytes, bundle signature is %d bytes",
			ErrSignatureMismatch, logIndex,
			len(body.Spec.Signature.Content), len(bundle.MessageSignature.Signature))
	}

	// Key material is compared only when both sides carry raw bytes. A cosign v3
	// key-based bundle may carry a Hint instead, which is not the same encoding
	// and is not guessed at here; the digest and signature comparisons above
	// already tie the entry to this artifact and this signature.
	if want := bundleRawKey(bundle); len(want) > 0 && len(body.Spec.Signature.PublicKey.Content) > 0 {
		if !bytes.Equal(body.Spec.Signature.PublicKey.Content, want) {
			return fmt.Errorf("%w: entry %d key is %d bytes, bundle key is %d bytes",
				ErrPublicKeyMismatch, logIndex, len(body.Spec.Signature.PublicKey.Content), len(want))
		}
	}

	return nil
}

// isSHA256 reports whether a hash-algorithm name from a Rekor body or a sigstore
// bundle denotes SHA-256. Rekor writes "sha256"; sigstore bundles write
// "SHA2_256".
func isSHA256(alg string) bool {
	switch strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(alg)) {
	case "sha256", "sha2256":
		return true
	}
	return false
}

// bundleRawKey returns the raw certificate or public key bytes a bundle carries,
// or nil when it carries neither (a hint-only bundle).
func bundleRawKey(bundle *Bundle) []byte {
	switch {
	case bundle.VerificationMaterial.Certificate != nil && len(bundle.VerificationMaterial.Certificate.RawBytes) > 0:
		return bundle.VerificationMaterial.Certificate.RawBytes
	case bundle.VerificationMaterial.PublicKey != nil && len(bundle.VerificationMaterial.PublicKey.RawBytes) > 0:
		return bundle.VerificationMaterial.PublicKey.RawBytes
	}
	return nil
}
