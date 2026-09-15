// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func frozenLockfileFixture() *spec.LockFile {
	return &spec.LockFile{
		ProfileName: "ml-env",
		Base:        spec.ResolvedBase{AMISHA256: strings.Repeat("a", 64)},
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "python-3.11-x86_64", Name: "python", Version: "3.11", SHA256: strings.Repeat("b", 64)}, MountOrder: 1},
		},
	}
}

// TestRunSign_RequiresFrozen: an unfrozen lockfile names no fixed set, so signing
// is refused before any key is touched.
func TestRunSign_RequiresFrozen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "unfrozen.lock.yaml")
	unfrozen := &spec.LockFile{
		ProfileName: "x",
		Layers:      []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{ID: "a-1-x86_64", Name: "a", Version: "1"}, MountOrder: 1}},
	}
	if err := writeYAML(p, unfrozen); err != nil {
		t.Fatal(err)
	}
	err := runSign(p, "unused")
	if err == nil || !strings.Contains(err.Error(), "not frozen") {
		t.Fatalf("expected a not-frozen refusal, got %v", err)
	}
}

// TestRunSign_SignsFrozen exercises the command wiring through the signLockFile
// seam with a fake signer (no AWS/cosign): a frozen lockfile is signed and the
// bundle + Rekor index are written back.
func TestRunSign_SignsFrozen(t *testing.T) {
	orig := signLockFile
	t.Cleanup(func() { signLockFile = orig })
	signLockFile = func(ctx context.Context, lf *spec.LockFile, _ string) error {
		return trust.SignLockFile(ctx, lf, &trust.FakeSigner{})
	}

	p := filepath.Join(t.TempDir(), "frozen.lock.yaml")
	if err := writeYAML(p, frozenLockfileFixture()); err != nil {
		t.Fatal(err)
	}
	if err := runSign(p, defaultSigningKey); err != nil {
		t.Fatalf("runSign: %v", err)
	}

	signed, err := spec.ParseLockFile(p)
	if err != nil {
		t.Fatalf("re-reading signed lockfile: %v", err)
	}
	if signed.Bundle == "" {
		t.Error("signed lockfile has no bundle")
	}
	if signed.RekorEntry == "" {
		t.Error("signed lockfile records no Rekor log index")
	}
	// And it round-trips: the signature verifies with the fake verifier.
	if err := trust.VerifyLockFile(context.Background(), signed, &trust.FakeVerifier{}); err != nil {
		t.Errorf("signed lockfile does not verify: %v", err)
	}
}

// TestLockfileSignatureFailures covers the verify-side wiring through the
// verifyLockfileSignature seam: an unsigned lockfile produces no failure, a
// freshly signed one verifies, and a tampered one is caught.
func TestLockfileSignatureFailures(t *testing.T) {
	orig := verifyLockfileSignature
	t.Cleanup(func() { verifyLockfileSignature = orig })
	verifyLockfileSignature = func(ctx context.Context, lf *spec.LockFile) error {
		return trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{})
	}

	if f := lockfileSignatureFailures(&spec.LockFile{}); len(f) != 0 {
		t.Errorf("unsigned lockfile produced signature failures: %v", f)
	}

	lf := frozenLockfileFixture()
	if err := trust.SignLockFile(context.Background(), lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("SignLockFile: %v", err)
	}
	if f := lockfileSignatureFailures(lf); len(f) != 0 {
		t.Errorf("freshly signed lockfile failed verification: %v", f)
	}

	lf.Layers[0].SHA256 = strings.Repeat("f", 64) // tamper after signing
	if f := lockfileSignatureFailures(lf); len(f) == 0 {
		t.Error("tampered lockfile passed signature verification")
	}
}
