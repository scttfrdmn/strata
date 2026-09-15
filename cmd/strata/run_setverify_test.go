// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// These tests pin the set-level verification strata run gained for #101: run
// verified each layer individually but never the lockfile as a set, so a
// mix-and-match of individually-valid, individually-signed layers — a set no
// maintainer attested — mounted with every per-layer check passing. verifyRunSet
// is the wrapper runRun's pre-mount step calls; the FakeSigner/FakeVerifier model
// the key+Rekor flow the same way internal/trust/lockfile_test.go does.
//
// Bound: chosen/implementation. The set-verification is driven through
// verifyRunSet with an injected verifier, matching how verifyRunSignatures is
// tested; the production wiring (verifyRunLayers building a real CosignVerifier)
// is exercised by the run integration test that reaches the layer checks.

func signedRunLockfile(t *testing.T) *spec.LockFile {
	t.Helper()
	lf := &spec.LockFile{
		ProfileName: "run-setverify-fixture",
		Base:        spec.ResolvedBase{AMISHA256: strings.Repeat("a", 64)},
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "python-3.11", Name: "python", Version: "3.11", SHA256: strings.Repeat("b", 64)}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{ID: "numpy-1.26", Name: "numpy", Version: "1.26", SHA256: strings.Repeat("c", 64)}, MountOrder: 2},
		},
	}
	if err := trust.SignLockFile(context.Background(), lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("SignLockFile: %v", err)
	}
	return lf
}

// TestVerifyRunSet_AcceptsSignedSet is the positive control: without it, a
// verifyRunSet that rejected everything would pass the rejection cases below.
func TestVerifyRunSet_AcceptsSignedSet(t *testing.T) {
	lf := signedRunLockfile(t)
	if failures := verifyRunSet(context.Background(), lf, &trust.FakeVerifier{}); len(failures) != 0 {
		t.Fatalf("verifyRunSet rejected a validly-signed set: %v", failures)
	}
}

// TestVerifyConsumedLockfile_NoVerifyAnnouncesTheSkip pins the escape hatch
// export and fold share: --no-verify must not fail, and it must say so on stderr
// rather than skip silently — a silent skip is indistinguishable from a check
// that ran and passed (#55). The embedded-key path is a production seam (needs
// cosign and a set signed by the embedded key) and is exercised end-to-end, not
// here.
func TestVerifyConsumedLockfile_NoVerifyAnnouncesTheSkip(t *testing.T) {
	// Deliberately unverifiable material: an unsigned lockfile with a bad bundle.
	// With noVerify the function must not look at it.
	lf := &spec.LockFile{Layers: []spec.ResolvedLayer{
		{LayerManifest: spec.LayerManifest{ID: "python-3.11", SHA256: strings.Repeat("a", 64)}},
	}}

	var err error
	stderr := captureStderr(t, func() {
		err = verifyConsumedLockfile(context.Background(), lf, []overlay.LayerPath{}, true)
	})
	if err != nil {
		t.Fatalf("--no-verify must not fail: %v", err)
	}
	if !strings.Contains(stderr, "NOT verified") {
		t.Errorf("--no-verify skipped verification silently; stderr was: %q", stderr)
	}
}

// TestVerifyRunSet_RefusesUnverifiableSets drives two rejection kinds that differ
// in kind, not degree: an unsigned set (the opt-in gap #101 names — an unsigned
// lockfile must be refused, not treated as trivially valid) and a set tampered
// after signing (the mix-and-match the set signature exists to catch). Both must
// be reported as a set-signature failure so runRun turns them into a refusal.
func TestVerifyRunSet_RefusesUnverifiableSets(t *testing.T) {
	cases := map[string]func(*spec.LockFile){
		"unsigned set: no bundle": func(lf *spec.LockFile) {
			lf.Bundle = ""
			lf.RekorEntry = ""
		},
		"mix-and-match: a layer appended after signing": func(lf *spec.LockFile) {
			lf.Layers = append(lf.Layers, spec.ResolvedLayer{
				LayerManifest: spec.LayerManifest{ID: "evil-1.0", Name: "evil", Version: "1.0", SHA256: strings.Repeat("e", 64)},
				MountOrder:    3,
			})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			lf := signedRunLockfile(t)
			mutate(lf)
			failures := verifyRunSet(context.Background(), lf, &trust.FakeVerifier{})
			if len(failures) == 0 {
				t.Fatalf("verifyRunSet accepted an unverifiable set (%s)", name)
			}
			if !strings.Contains(failures[0], "set signature") {
				t.Errorf("failure does not name the set-signature check: %q", failures[0])
			}
		})
	}
}
