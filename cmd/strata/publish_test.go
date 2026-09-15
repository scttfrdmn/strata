// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// A 64-hex string so the fixtures are frozen under IsFrozen.
const publishTestDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// writePublishFixture marshals lf to a temp file and returns the path.
func writePublishFixture(t *testing.T, lf spec.LockFile) string {
	t.Helper()
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatalf("marshal lockfile: %v", err)
	}
	p := filepath.Join(t.TempDir(), "lock.yaml")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}
	return p
}

// useFakeVerifier points the lockfile-signature seam at a fake verifier for the
// test, so a fixture signed with FakeSigner verifies without cosign or the
// network. publish now verifies the signature, not just its presence (#60).
func useFakeVerifier(t *testing.T) {
	t.Helper()
	orig := verifyLockfileSignature
	t.Cleanup(func() { verifyLockfileSignature = orig })
	verifyLockfileSignature = func(ctx context.Context, lf *spec.LockFile) error {
		return trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{})
	}
}

// publishableLockFile is frozen, clean and signed — the only shape publish may
// accept. It is signed with FakeSigner so it carries a real bundle the fake
// verifier accepts. Each rejection test flips exactly one property off, so a
// refusal is attributable to that single field; the accepts-test is the
// non-vacuity anchor that the base fixture is otherwise publishable.
func publishableLockFile(t *testing.T) spec.LockFile {
	t.Helper()
	lf := spec.LockFile{
		Base:   spec.ResolvedBase{AMISHA256: publishTestDigest},
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{ID: "tool-1.0-x86_64", Name: "tool", Version: "1.0", SHA256: publishTestDigest}, MountOrder: 1}},
	}
	if err := trust.SignLockFile(context.Background(), &lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("signing fixture: %v", err)
	}
	return lf
}

func TestParsePublishableLockFile_AcceptsFrozenCleanSigned(t *testing.T) {
	useFakeVerifier(t)
	if _, err := parsePublishableLockFile(writePublishFixture(t, publishableLockFile(t))); err != nil {
		t.Fatalf("a frozen, clean, signed lockfile must be publishable, got: %v", err)
	}
}

func TestParsePublishableLockFile_RejectsUnfrozen(t *testing.T) {
	useFakeVerifier(t)
	lf := publishableLockFile(t)
	lf.Base.AMISHA256 = ""
	assertPublishRejected(t, lf, "not frozen")
}

func TestParsePublishableLockFile_RejectsDirtyEnvironment(t *testing.T) {
	useFakeVerifier(t)
	lf := publishableLockFile(t)
	lf.MutableLayer = &spec.MutableLayerSpec{Name: "dirty-upper", Version: "0.1.0"}
	assertPublishRejected(t, lf, "mutable upper layer")
}

func TestParsePublishableLockFile_RejectsUnsigned(t *testing.T) {
	useFakeVerifier(t)
	lf := publishableLockFile(t)
	lf.Bundle = ""
	lf.RekorEntry = ""
	assertPublishRejected(t, lf, "not signed")
}

// TestParsePublishableLockFile_RejectsInvalidSignature is the case IsSigned()
// could not catch: a lockfile that carries a signature but has been altered
// since. publish must refuse it rather than mint a DOI over a tampered set (#60).
func TestParsePublishableLockFile_RejectsInvalidSignature(t *testing.T) {
	useFakeVerifier(t)
	lf := publishableLockFile(t)
	lf.Layers[0].SHA256 = strings.Repeat("f", 64) // tamper after signing
	assertPublishRejected(t, lf, "does not verify")
}

func assertPublishRejected(t *testing.T, lf spec.LockFile, wantReason string) {
	t.Helper()
	_, err := parsePublishableLockFile(writePublishFixture(t, lf))
	if err == nil {
		t.Fatalf("expected publish to refuse the lockfile, got nil error")
	}
	if !strings.Contains(err.Error(), wantReason) {
		t.Fatalf("refusal must name the reason %q, got: %v", wantReason, err)
	}
}
