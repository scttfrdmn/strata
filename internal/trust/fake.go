package trust

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// FakeSigner is a deterministic Signer for testing. It produces bundles
// containing a SHA256-based "signature" with a predictable Rekor log index.
// FakeSigner is not cryptographically secure and must never be used in
// production.
type FakeSigner struct {
	// NextLogIndex is incremented for each Sign call.
	NextLogIndex int64
}

// Sign returns a Bundle whose signature is the SHA256 of the artifact,
// with a fake Rekor entry at the current NextLogIndex.
func (s *FakeSigner) Sign(_ context.Context, artifactPath string, annotations map[string]string) (*Bundle, error) {
	digest, err := sha256FileBytes(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("FakeSigner: hashing %q: %w", artifactPath, err)
	}

	_ = annotations // included for interface compliance

	idx := s.NextLogIndex
	s.NextLogIndex++

	return &Bundle{
		MediaType: BundleMediaType,
		VerificationMaterial: VerificationMaterial{
			TlogEntries: []TlogEntry{
				{
					LogIndex:       fmt.Sprintf("%d", idx),
					LogID:          "fake-rekor-log-id",
					IntegratedTime: "0",
				},
			},
		},
		MessageSignature: MessageSignature{
			MessageDigest: MessageDigest{
				Algorithm: "SHA2_256",
				Digest:    digest,
			},
			// Signature = hex of digest as a stand-in for real sig bytes.
			Signature: []byte(hex.EncodeToString(digest)),
		},
	}, nil
}

// FakeVerifier is a deterministic Verifier for testing. It accepts any bundle
// produced by FakeSigner (signature = SHA256 of artifact) and rejects all others.
// FakeVerifier is not cryptographically secure and must never be used in production.
type FakeVerifier struct{}

// Verify succeeds iff the bundle's MessageSignature.Signature equals the
// hex SHA256 of artifactPath — matching what FakeSigner produces.
func (v *FakeVerifier) Verify(_ context.Context, artifactPath string, bundle *Bundle) error {
	if !bundle.HasRekorEntry() {
		return fmt.Errorf("FakeVerifier: bundle has no Rekor entry")
	}

	digest, err := sha256FileBytes(artifactPath)
	if err != nil {
		return fmt.Errorf("FakeVerifier: hashing %q: %w", artifactPath, err)
	}

	want := []byte(hex.EncodeToString(digest))
	if string(bundle.MessageSignature.Signature) != string(want) {
		return fmt.Errorf("FakeVerifier: signature mismatch for %q", artifactPath)
	}
	return nil
}

// FakeRekorClient is a no-op RekorClient for testing. Log always succeeds
// and increments a counter. VerifyEntry always succeeds.
type FakeRekorClient struct {
	// LoggedBundles records every bundle passed to Log.
	LoggedBundles []*Bundle
	nextIndex     int64
}

// Log records the bundle and returns the next log index.
func (c *FakeRekorClient) Log(_ context.Context, bundle *Bundle) (int64, error) {
	c.LoggedBundles = append(c.LoggedBundles, bundle)
	idx := c.nextIndex
	c.nextIndex++
	return idx, nil
}

// VerifyEntry always succeeds for testing purposes.
func (c *FakeRekorClient) VerifyEntry(_ context.Context, _ int64, _ *Bundle) error {
	return nil
}

// sha256FileBytes returns the raw SHA256 bytes of the named file's contents.
func sha256FileBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
