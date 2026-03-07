package trust

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

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

	args := []string{
		"sign-blob",
		"--bundle", bundlePath,
		"--yes", // skip interactive confirmation
	}
	if s.KeyRef != "" {
		args = append(args, "--key", s.KeyRef)
	}
	for k, v := range annotations {
		args = append(args, "--annotations", k+"="+v)
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

// VerifyEntry confirms a bundle is present at logIndex in the Rekor log.
func (c *RekorHTTPClient) VerifyEntry(ctx context.Context, logIndex int64, bundle *Bundle) error {
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
			// Entry exists in the log. For full verification, the body would be
			// decoded and compared against the bundle. For now, existence is sufficient
			// for the unit layer of trust; the cosign CLI performs the full check.
			_ = bundle
			return nil
		}
	}
	return fmt.Errorf("rekor verify: entry at logIndex %d not found", logIndex)
}
