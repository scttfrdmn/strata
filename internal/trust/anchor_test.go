// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package trust_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
)

// TestEmbeddedCosignKey asserts the shared trust anchor is a real key: the bytes
// embedded from keys/cosign.pub are non-empty and parse as a PEM-encoded public
// key. A release that shipped an empty or malformed embed would fail closed at
// every verification boundary, so this catches it at build/test time. (The
// separate fact that this key is the KMS signing key's public half is verified
// operationally against `aws kms get-public-key`, #62.)
func TestEmbeddedCosignKey(t *testing.T) {
	k := trust.EmbeddedCosignKey()
	if len(bytes.TrimSpace(k)) == 0 {
		t.Fatal("embedded cosign key is empty — no trust anchor")
	}
	block, _ := pem.Decode(k)
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

// TestWriteEmbeddedKeyFile asserts the pinned key is materialised verbatim to the
// temp file cosign consumes, and cleaned up on request.
func TestWriteEmbeddedKeyFile(t *testing.T) {
	path, cleanup, err := trust.WriteEmbeddedKeyFile()
	if err != nil {
		t.Fatalf("WriteEmbeddedKeyFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written key: %v", err)
	}
	if !bytes.Equal(got, trust.EmbeddedCosignKey()) {
		t.Error("written key file does not match the embedded bytes")
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove the temp key file: %v", err)
	}
}

// TestEmbeddedKeyVerifier asserts a verifier is returned and it carries the
// embedded key as its reference.
func TestEmbeddedKeyVerifier(t *testing.T) {
	v, cleanup, err := trust.EmbeddedKeyVerifier()
	if err != nil {
		t.Fatalf("EmbeddedKeyVerifier: %v", err)
	}
	defer cleanup()
	cv, ok := v.(*trust.CosignVerifier)
	if !ok {
		t.Fatalf("want *trust.CosignVerifier, got %T", v)
	}
	got, err := os.ReadFile(cv.KeyRef)
	if err != nil {
		t.Fatalf("reading verifier key: %v", err)
	}
	if !bytes.Equal(got, trust.EmbeddedCosignKey()) {
		t.Error("verifier key is not the embedded key")
	}
}
