package trust_test

// Corpus for RekorHTTPClient.VerifyEntry (#59).
//
// A verifier that verifies nothing and a verifier that verifies correctly both
// return success on honest input, so honest input cannot tell them apart. What
// distinguishes them is material a correct implementation rejects — and a
// rejection that fires from a different check than the one under test looks
// identical in the output. So every case below asserts the *specific* sentinel
// error, never merely "an error occurred".
//
// The server is a local httptest stub, not rekor.sigstore.dev: this runs in CI's
// Test job, offline, with no dependence on the contents of the public log. The
// entry-body shape it serves is the one this repository's own Log method writes
// (internal/trust/cosign.go). That is an assumption about Rekor's canonicalized
// hashedrekord encoding, not a measurement of it.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/internal/trust"
)

// ---- stub Rekor ----

// rekorStub serves GET /api/v1/log/entries?logIndex=N from a fixed table of
// base64 entry bodies. An index absent from the table produces an empty object,
// which is what the real API returns for an index that does not exist.
func rekorStub(t *testing.T, bodies map[int64]string) *trust.RekorHTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx, err := strconv.ParseInt(r.URL.Query().Get("logIndex"), 10, 64)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			http.Error(w, "bad logIndex", http.StatusBadRequest)
			return
		}
		body, ok := bodies[idx]
		if !ok {
			fmt.Fprint(w, `{}`) //nolint:errcheck
			return
		}
		fmt.Fprintf(w, `{"%040x":{"logIndex":%d,"body":%q}}`, idx, idx, body) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return &trust.RekorHTTPClient{BaseURL: srv.URL}
}

// ---- entry bodies ----

// entryBody is the hashedrekord shape RekorHTTPClient.Log writes and
// VerifyEntry reads. Declared here rather than reused because the production
// type is unexported: a test that shares the encoder with the code under test
// cannot detect an encoding disagreement, and this one is meant to.
type entryBody struct {
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

// body builds a base64-encoded entry body.
func body(t *testing.T, kind, algorithm, hexDigest string, sig, key []byte) string {
	t.Helper()
	var b entryBody
	b.APIVersion = "0.0.1"
	b.Kind = kind
	b.Spec.Data.Hash.Algorithm = algorithm
	b.Spec.Data.Hash.Value = hexDigest
	b.Spec.Signature.Content = sig
	b.Spec.Signature.PublicKey.Content = key

	raw, err := json.Marshal(&b)
	if err != nil {
		t.Fatalf("marshalling entry body: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// ---- bundles ----

// testBundle builds a bundle claiming logIndex over digest, signed by sig.
func testBundle(logIndex int64, digest, sig, key []byte) *trust.Bundle {
	b := &trust.Bundle{MediaType: trust.BundleMediaType}
	b.VerificationMaterial.TlogEntries = []trust.TlogEntry{{
		LogIndex:    strconv.FormatInt(logIndex, 10),
		KindVersion: &trust.KindVersion{Kind: "hashedrekord", Version: "0.0.1"},
	}}
	b.MessageSignature.MessageDigest.Algorithm = "SHA2_256"
	b.MessageSignature.MessageDigest.Digest = digest
	b.MessageSignature.Signature = sig
	if len(key) > 0 {
		b.VerificationMaterial.PublicKey = &trust.RawMaterial{RawBytes: key}
	}
	return b
}

func sha256Of(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// ---- the corpus ----

// TestVerifyEntry_Corpus drives one input per rejection reason plus the honest
// case. The wantErr column is the whole point: it asserts *which* check fired.
func TestVerifyEntry_Corpus(t *testing.T) {
	const idx int64 = 42

	artifact := sha256Of("the artifact this bundle attests")
	stranger := sha256Of("somebody else's artifact, logged at the same index")
	sig := []byte("signature-over-the-artifact-digest")
	otherSig := []byte("a different signature entirely......")
	key := []byte("public-key-raw-bytes")
	otherKey := []byte("a different public key")

	hexArtifact := hex.EncodeToString(artifact)

	cases := []struct {
		name    string
		bodies  map[int64]string
		bundle  *trust.Bundle
		wantErr error // nil means the entry must verify
		why     string
	}{
		{
			name:    "honest: entry built from the bundle",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, sig, key)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: nil,
			why:     "digest, signature and key all agree",
		},
		{
			name:    "no bundle supplied",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, sig, key)},
			bundle:  nil,
			wantErr: trust.ErrNoBundle,
			why:     "an index with no bundle is unverifiable; this is the pre-#59 success path",
		},
		{
			name:    "stranger's entry: valid, findable, different artifact",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hex.EncodeToString(stranger), sig, key)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrDigestMismatch,
			why:     "the case the old existence check passed: borrowing somebody else's attestation",
		},
		{
			name:    "same artifact, different signature",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, otherSig, key)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrSignatureMismatch,
			why:     "digest agreement alone does not tie the entry to this signature",
		},
		{
			name:    "same artifact and signature, different key",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, sig, otherKey)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrPublicKeyMismatch,
			why:     "key material is compared when both sides carry raw bytes",
		},
		{
			name:    "bundle names a different log index",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, sig, key)},
			bundle:  testBundle(idx+1, artifact, sig, key),
			wantErr: trust.ErrLogIndexMismatch,
			why:     "a bundle about another entry is not evidence about this one",
		},
		{
			name:    "no entry at the index",
			bodies:  map[int64]string{},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrEntryNotFound,
			why:     "absence must not read as agreement",
		},
		{
			name:    "entry is not a hashedrekord",
			bodies:  map[int64]string{idx: body(t, "intoto", "sha256", hexArtifact, sig, key)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrUnsupportedEntryKind,
			why:     "an entry this client cannot compare must not pass as compared",
		},
		{
			name:    "entry body is not base64",
			bodies:  map[int64]string{idx: "!!!not base64!!!"},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrMalformedEntry,
			why:     "an undecodable body is not a verified body",
		},
		{
			name:    "entry body is base64 of non-JSON",
			bodies:  map[int64]string{idx: base64.StdEncoding.EncodeToString([]byte("not json"))},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrMalformedEntry,
			why:     "same, one layer in",
		},
		{
			name:    "empty bundle digest and signature against an empty entry",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", "", nil, nil)},
			bundle:  testBundle(idx, nil, nil, nil),
			wantErr: trust.ErrIncompleteBundle,
			why:     "empty == empty would otherwise verify, which is how this class of defect is written",
		},
		{
			name:    "entry names a non-SHA256 algorithm",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha512", hexArtifact, sig, key)},
			bundle:  testBundle(idx, artifact, sig, key),
			wantErr: trust.ErrDigestAlgorithm,
			why:     "a digest comparison across algorithms is not a comparison",
		},
		{
			name:    "hint-only bundle: key comparison is documented as skipped",
			bodies:  map[int64]string{idx: body(t, "hashedrekord", "sha256", hexArtifact, sig, otherKey)},
			bundle:  hintOnlyBundle(idx, artifact, sig),
			wantErr: nil,
			why:     "documented limit, asserted rather than left implicit: digest and signature still tie the entry to the artifact",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := rekorStub(t, tc.bodies)
			err := c.VerifyEntry(context.Background(), idx, tc.bundle)

			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("VerifyEntry() = %v, want nil (%s)", err, tc.why)
			case tc.wantErr == nil:
				t.Logf("verified: %s", tc.why)
			case err == nil:
				t.Fatalf("VerifyEntry() = nil, want %v (%s)", tc.wantErr, tc.why)
			case !errors.Is(err, tc.wantErr):
				// The point of the sentinels: a rejection for the wrong reason
				// is a failure, not a pass.
				t.Fatalf("VerifyEntry() rejected for the wrong reason:\n  got:  %v\n  want: %v\n  (%s)",
					err, tc.wantErr, tc.why)
			default:
				t.Logf("rejected as %v — %s", tc.wantErr, tc.why)
			}
		})
	}
}

// hintOnlyBundle is a cosign v3 key-based bundle that carries a key hint rather
// than raw key bytes, so there is nothing for the key comparison to compare.
func hintOnlyBundle(logIndex int64, digest, sig []byte) *trust.Bundle {
	b := testBundle(logIndex, digest, sig, nil)
	b.VerificationMaterial.PublicKey = &trust.RawMaterial{Hint: "aGludA=="}
	return b
}

// TestVerifyEntry_DiscardsNothing is the mechanical form of the issue's closing
// condition: the bundle parameter must be able to change the outcome. Two calls
// differing only in the bundle must not both succeed.
func TestVerifyEntry_DiscardsNothing(t *testing.T) {
	const idx int64 = 7
	mine := sha256Of("mine")
	theirs := sha256Of("theirs")
	sig := []byte("sig")

	c := rekorStub(t, map[int64]string{
		idx: body(t, "hashedrekord", "sha256", hex.EncodeToString(mine), sig, nil),
	})

	if err := c.VerifyEntry(context.Background(), idx, testBundle(idx, mine, sig, nil)); err != nil {
		t.Fatalf("matching bundle: VerifyEntry() = %v, want nil", err)
	}
	err := c.VerifyEntry(context.Background(), idx, testBundle(idx, theirs, sig, nil))
	if err == nil {
		t.Fatal("unrelated bundle verified against the same log entry: the bundle is still being discarded")
	}
	if !errors.Is(err, trust.ErrDigestMismatch) {
		t.Fatalf("unrelated bundle rejected for the wrong reason: %v", err)
	}
}

// ---- the repo's own fixture, consumed by a real verifier for the first time ----

// TestVerifyEntry_RejectsRepoFixture drives internal/testregistry's committed
// bundles through VerifyEntry.
//
// The fixture was built for this issue and its package doc predicts the step each
// bundle fails at. Nothing had ever run a real verifier over it, so the
// prediction was unmeasured. The two bundle kinds fail for *different* documented
// reasons and the test asserts each separately:
//
//   - layer bundles carry the real SHA-256 of layer.sqfs, "so a digest comparison
//     passes and the signature check is what fails";
//   - the formation bundle carries a zero digest, so it fails one step earlier.
//
// The stub serves, at the fixture's own log index, an entry that is honest about
// the artifact — which is the strongest form of the historical hazard: the
// fixture's first version carried plausible indices that really did resolve in
// the public log, to real entries for other people's artifacts.
func TestVerifyEntry_RejectsRepoFixture(t *testing.T) {
	root, err := testregistry.Materialize(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}

	idx, err := strconv.ParseInt(testregistry.SentinelRekorEntry, 10, 64)
	if err != nil {
		t.Fatalf("SentinelRekorEntry is not an int64: %v", err)
	}

	layers, formations := findFixtureBundles(t, root)
	if len(layers) == 0 || len(formations) == 0 {
		t.Fatalf("fixture bundles not found: %d layer, %d formation", len(layers), len(formations))
	}

	for _, path := range layers {
		t.Run("layer/"+filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			bundle := parseBundleFile(t, path)

			// Measured, not assumed: the doc claim is that the digest is the real
			// SHA-256 of the sibling layer.sqfs. Without this, "the digest step
			// passes" below would be true only because the stub entry was built
			// from the bundle's own digest — a tautology dressed as a measurement.
			artifact, err := os.ReadFile(filepath.Join(filepath.Dir(path), "layer.sqfs"))
			if err != nil {
				t.Fatalf("reading the artifact this bundle attests: %v", err)
			}
			realSum := sha256.Sum256(artifact)
			if !bytes.Equal(realSum[:], bundle.MessageSignature.MessageDigest.Digest) {
				t.Fatalf("fixture bundle digest is not the digest of layer.sqfs:\n  bundle: %x\n  file:   %x\n"+
					"internal/testregistry's package doc claims it is, and the signature-step "+
					"assertion below depends on it", bundle.MessageSignature.MessageDigest.Digest, realSum[:])
			}

			// An entry that agrees with the fixture about the artifact — that same
			// real digest — and carries a plausible signature over it. Everything a
			// presence check can see is in order.
			digest := hex.EncodeToString(bundle.MessageSignature.MessageDigest.Digest)
			c := rekorStub(t, map[int64]string{
				idx: body(t, "hashedrekord", "sha256", digest, sha256Of("a real signature, not the fixture's"), nil),
			})

			err = c.VerifyEntry(context.Background(), idx, bundle)
			if err == nil {
				t.Fatal("fixture layer bundle verified against a transparency-log entry: " +
					"a fixture that can pass real verification can launder a stranger's attestation")
			}
			if !errors.Is(err, trust.ErrSignatureMismatch) {
				t.Fatalf("fixture layer bundle rejected, but not at the step its package doc claims:\n"+
					"  got:  %v\n  want: %v", err, trust.ErrSignatureMismatch)
			}
			t.Logf("rejected at the signature step, as internal/testregistry's package doc predicts: %v", err)
		})
	}

	for _, path := range formations {
		t.Run("formation/"+filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			bundle := parseBundleFile(t, path)

			c := rekorStub(t, map[int64]string{
				idx: body(t, "hashedrekord", "sha256", hex.EncodeToString(sha256Of("some real artifact")),
					sha256Of("a real signature"), nil),
			})

			err := c.VerifyEntry(context.Background(), idx, bundle)
			if err == nil {
				t.Fatal("fixture formation bundle verified against a transparency-log entry")
			}
			// The formation bundle's messageDigest is 32 zero bytes, not the
			// digest of anything, so the digest comparison is what rejects it.
			if !errors.Is(err, trust.ErrDigestMismatch) {
				t.Fatalf("fixture formation bundle rejected for an unexpected reason:\n  got:  %v\n  want: %v",
					err, trust.ErrDigestMismatch)
			}
			t.Logf("rejected at the digest step: %v", err)
		})
	}
}

// TestVerifyEntry_FixtureIndexIsUnfindable pins the other half of the fixture's
// design: its log index cannot name a real entry, so against a log that does not
// hold it the rejection is ErrEntryNotFound and never a pass.
func TestVerifyEntry_FixtureIndexIsUnfindable(t *testing.T) {
	root, err := testregistry.Materialize(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}
	layers, _ := findFixtureBundles(t, root)
	bundle := parseBundleFile(t, layers[0])

	idx, err := strconv.ParseInt(testregistry.SentinelRekorEntry, 10, 64)
	if err != nil {
		t.Fatalf("SentinelRekorEntry is not an int64: %v", err)
	}

	// An empty log: every index is absent.
	c := rekorStub(t, map[int64]string{})
	err = c.VerifyEntry(context.Background(), idx, bundle)
	if !errors.Is(err, trust.ErrEntryNotFound) {
		t.Fatalf("VerifyEntry() = %v, want %v", err, trust.ErrEntryNotFound)
	}
}

// findFixtureBundles returns the layer and formation bundle paths under a
// materialized fixture registry, discovered rather than hardcoded so that a
// fixture gaining a layer is covered without editing this test.
func findFixtureBundles(t *testing.T, root string) (layers, formations []string) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "bundle.json" {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		switch {
		case strings.HasPrefix(rel, "layers"+string(filepath.Separator)):
			layers = append(layers, p)
		case strings.HasPrefix(rel, "formations"+string(filepath.Separator)):
			formations = append(formations, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking fixture registry: %v", err)
	}
	return layers, formations
}

func parseBundleFile(t *testing.T, path string) *trust.Bundle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	b, err := trust.ParseBundle(data)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return b
}
