package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
)

// TestWriteEmbeddedCosignKey asserts the agent's fetchKey wrapper materialises
// the shared embedded key (internal/trust) verbatim — the path CosignVerifier
// consumes. The key's validity and the KMS correspondence are tested in
// internal/trust; this covers the agent's thin wrapper.
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
	if !bytes.Equal(got, trust.EmbeddedCosignKey()) {
		t.Error("written key file does not match the embedded bytes")
	}
}

// TestProductionVerifierUsesTheEmbeddedKey closes the wiring gap the existence
// tests leave: it goes through productionPrereqs (real fetchKey), overrides only
// lookPath so the key path is reached without cosign installed, and asserts the
// verifier's KeyRef bytes are the embedded key. If a registry or KMS fetch were
// reintroduced and wired into productionPrereqs.fetchKey, this reddens while the
// existence tests stay green (#62).
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
	if !bytes.Equal(got, trust.EmbeddedCosignKey()) {
		t.Error("the production verifier's key is not the embedded key — a registry or KMS fetch may have been wired into productionPrereqs.fetchKey")
	}
}
