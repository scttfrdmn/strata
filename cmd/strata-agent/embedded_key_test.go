package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
)

// TestEmbeddedCosignKey asserts the agent ships a real trust anchor: the key
// embedded from keys/cosign.pub is non-empty and parses as a PEM-encoded public
// key. #62 pins the key in the binary; a release that shipped an empty or
// malformed embed would fail closed at boot (writeEmbeddedCosignKey returns "",
// which resolveVerifier turns into a refusal), so this catches it at build/test
// time rather than on an instance that will not boot.
func TestEmbeddedCosignKey(t *testing.T) {
	if len(bytes.TrimSpace(embeddedCosignKey)) == 0 {
		t.Fatal("embedded cosign key is empty — the agent has no trust anchor")
	}
	block, _ := pem.Decode(embeddedCosignKey)
	if block == nil {
		t.Fatal("embedded cosign key is not PEM-encoded")
	}
	if block.Type != "PUBLIC KEY" {
		t.Errorf("embedded key PEM type = %q, want %q", block.Type, "PUBLIC KEY")
	}
	if _, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		t.Errorf("embedded cosign key does not parse as a public key: %v", err)
	}
}

// TestWriteEmbeddedCosignKey asserts the pinned key is materialised to a file
// whose bytes are exactly the embed — the path CosignVerifier.KeyRef consumes.
func TestWriteEmbeddedCosignKey(t *testing.T) {
	path := writeEmbeddedCosignKey(context.Background())
	if path == "" {
		t.Fatal(`writeEmbeddedCosignKey returned "" with a non-empty embed`)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written key: %v", err)
	}
	if !bytes.Equal(got, embeddedCosignKey) {
		t.Error("written key file does not match the embedded bytes")
	}
}

// TestProductionVerifierUsesTheEmbeddedKey closes the wiring gap the existence
// tests leave: they prove an embedded key exists and is usable, not that
// production actually verifies against it. This goes through productionPrereqs
// (whose fetchKey is the real writeEmbeddedCosignKey), overriding only lookPath
// so the key path is reached without cosign installed, then reads the verifier's
// KeyRef and asserts its bytes are the embedded key. If a registry or KMS fetch
// were reintroduced and wired into productionPrereqs.fetchKey, this reddens
// while the existence tests stay green (#62).
func TestProductionVerifierUsesTheEmbeddedKey(t *testing.T) {
	p := productionPrereqs(func(string) string { return "" })
	p.lookPath = func(string) (string, error) { return "/usr/local/bin/cosign", nil }

	v, err := newCosignVerifier(context.Background(), p)
	if err != nil {
		t.Fatalf("newCosignVerifier through production wiring: %v", err)
	}
	cv, ok := v.(*trust.CosignVerifier)
	if !ok {
		t.Fatalf("want *trust.CosignVerifier, got %T", v)
	}
	t.Cleanup(func() { _ = os.Remove(cv.KeyRef) })

	got, err := os.ReadFile(cv.KeyRef)
	if err != nil {
		t.Fatalf("reading the verifier's key file: %v", err)
	}
	if !bytes.Equal(got, embeddedCosignKey) {
		t.Error("the production verifier's key is not the embedded key — a registry or KMS fetch may have been wired into productionPrereqs.fetchKey")
	}
}
