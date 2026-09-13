package trust

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestHashedRekorBody_ParsesRealEntry pins the hashedrekord wire format against a
// real entry fetched from the public Rekor log
// (testdata/rekor_hashedrekord_logindex_200000000.json).
//
// #88: the VerifyEntry corpus's notion of the format was derived from the code
// under test — Log writes hashedRekorBody and the stub echoes the same shape, so
// writer and verifier could agree on a *wrong* format and every test would still
// pass. This test parses an entry this repository did not produce, so a drift in
// any json tag or in an encoding assumption (hex for the digest, base64 for the
// signature and key) fails here rather than passing self-consistently.
//
// It reads a committed fixture, never the network: CI's offline jobs must not
// depend on the live log (#88 acceptance).
func TestHashedRekorBody_ParsesRealEntry(t *testing.T) {
	raw, err := os.ReadFile("testdata/rekor_hashedrekord_logindex_200000000.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	bodyJSON, err := base64.StdEncoding.DecodeString(fixture.Body)
	if err != nil {
		t.Fatalf("fixture body is not base64: %v", err)
	}

	var body hashedRekorBody
	if err := json.Unmarshal(bodyJSON, &body); err != nil {
		t.Fatalf("hashedRekorBody cannot parse a real Rekor entry: %v", err)
	}

	// json.Unmarshal is lenient: a renamed tag leaves the Go field zero-valued
	// with no error. So each assertion checks an expected value, and a non-empty
	// or exact match is the evidence the tag still matches the wire.
	if body.Kind != "hashedrekord" {
		t.Errorf("Kind = %q, want \"hashedrekord\" (a wrong `kind` tag leaves this empty)", body.Kind)
	}
	if body.APIVersion == "" {
		t.Error("APIVersion is empty — the `apiVersion` tag no longer matches the wire")
	}
	if body.Spec.Data.Hash.Algorithm != "sha256" {
		t.Errorf("spec.data.hash.algorithm = %q, want \"sha256\"", body.Spec.Data.Hash.Algorithm)
	}
	// hash.value is a sha256 hex digest; reuse the same validator the rest of the
	// module uses so the wire-format assumption and the digest gate cannot drift
	// apart.
	if err := spec.ValidateLayerDigest(body.Spec.Data.Hash.Value); err != nil {
		t.Errorf("spec.data.hash.value is not 64 lowercase hex (our encoding assumption): %v", err)
	}
	// Content and PublicKey.Content are []byte fields; encoding/json decodes a
	// base64 string into bytes, so a non-empty slice proves both the tag and the
	// base64 assumption.
	if len(body.Spec.Signature.Content) == 0 {
		t.Error("spec.signature.content decoded empty — tag drift or not base64")
	}
	if len(body.Spec.Signature.PublicKey.Content) == 0 {
		t.Error("spec.signature.publicKey.content decoded empty — tag drift or not base64")
	}
}
