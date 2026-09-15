package trust_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func signedTestLockfile(t *testing.T) *spec.LockFile {
	t.Helper()
	return &spec.LockFile{
		ProfileName: "ml-env",
		Base:        spec.ResolvedBase{AMISHA256: strings.Repeat("a", 64)},
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "python-3.11-x86_64", Name: "python", Version: "3.11", SHA256: strings.Repeat("b", 64)}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{ID: "numpy-1.26-x86_64", Name: "numpy", Version: "1.26", SHA256: strings.Repeat("c", 64)}, MountOrder: 2},
		},
	}
}

// TestSignAndVerifyLockFile_RoundTrip: a freshly signed lockfile verifies. The
// FakeSigner/FakeVerifier model the key+Rekor flow — the bundle carries a
// transparency-log entry (HasRekorEntry) and a signature over the payload.
func TestSignAndVerifyLockFile_RoundTrip(t *testing.T) {
	ctx := context.Background()
	lf := signedTestLockfile(t)

	if err := trust.SignLockFile(ctx, lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("SignLockFile: %v", err)
	}
	if lf.Bundle == "" {
		t.Fatal("SignLockFile did not record a bundle")
	}
	if lf.RekorEntry == "" {
		t.Fatal("SignLockFile did not record a transparency-log index")
	}
	if err := trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{}); err != nil {
		t.Fatalf("VerifyLockFile of a freshly signed lockfile: %v", err)
	}
}

// TestVerifyLockFile_RejectsTamper is the substitution case: after signing, any
// change to a field the signature covers must fail verification.
func TestVerifyLockFile_RejectsTamper(t *testing.T) {
	ctx := context.Background()

	mutations := map[string]func(*spec.LockFile){
		"swap a layer digest": func(lf *spec.LockFile) { lf.Layers[0].SHA256 = strings.Repeat("d", 64) },
		"reorder the set":     func(lf *spec.LockFile) { lf.Layers[0], lf.Layers[1] = lf.Layers[1], lf.Layers[0] },
		"add a layer (mix-and-match)": func(lf *spec.LockFile) {
			lf.Layers = append(lf.Layers, spec.ResolvedLayer{
				LayerManifest: spec.LayerManifest{ID: "evil-1.0-x86_64", Name: "evil", Version: "1.0", SHA256: strings.Repeat("e", 64)},
				MountOrder:    3,
			})
		},
		"change the base":         func(lf *spec.LockFile) { lf.Base.AMISHA256 = strings.Repeat("f", 64) },
		"change resolved profile": func(lf *spec.LockFile) { lf.ProfileName = "attacker-env" },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			lf := signedTestLockfile(t)
			if err := trust.SignLockFile(ctx, lf, &trust.FakeSigner{}); err != nil {
				t.Fatalf("SignLockFile: %v", err)
			}
			mutate(lf)
			if err := trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{}); err == nil {
				t.Errorf("VerifyLockFile accepted a lockfile tampered after signing (%s)", name)
			}
		})
	}
}

// TestVerifyLockFile_RejectsRekorEntryMismatch: RekorEntry is excluded from the
// signed payload, so the set signature keeps verifying if only RekorEntry is
// altered — but it is the lockfile's public transparency pointer, so verification
// must still reject a value that disagrees with the signed bundle's own log index.
func TestVerifyLockFile_RejectsRekorEntryMismatch(t *testing.T) {
	ctx := context.Background()
	lf := signedTestLockfile(t)
	if err := trust.SignLockFile(ctx, lf, &trust.FakeSigner{NextLogIndex: 42}); err != nil {
		t.Fatalf("SignLockFile: %v", err)
	}
	if lf.RekorEntry != "42" {
		t.Fatalf("SignLockFile recorded rekor_entry %q, want 42", lf.RekorEntry)
	}

	lf.RekorEntry = "999999" // falsify only the transparency pointer

	if err := trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{}); err == nil {
		t.Fatal("VerifyLockFile accepted a lockfile whose rekor_entry does not match its bundle")
	} else if !strings.Contains(err.Error(), "rekor_entry") {
		t.Errorf("error does not name the rekor_entry mismatch: %v", err)
	}
}

// TestVerifyLockFile_Unsigned: a lockfile with no bundle is refused, not treated
// as trivially valid.
func TestVerifyLockFile_Unsigned(t *testing.T) {
	err := trust.VerifyLockFile(context.Background(), signedTestLockfile(t), &trust.FakeVerifier{})
	if err == nil {
		t.Fatal("VerifyLockFile accepted an unsigned lockfile")
	}
	if !strings.Contains(err.Error(), "not signed") {
		t.Errorf("error does not name the missing signature: %v", err)
	}
}

// TestSignLockFile_SignatureFieldsAreNotSelfCovered: the payload excludes Bundle
// and RekorEntry, so re-signing (which rewrites them) does not invalidate the
// signature over the rest — and verification of a signed lockfile does not depend
// on those fields matching what was present at signing time beyond the bundle
// itself. This is the non-vacuity check that the payload elision is real: a
// lockfile signed, then re-signed, still verifies.
func TestSignLockFile_SignatureFieldsAreNotSelfCovered(t *testing.T) {
	ctx := context.Background()
	lf := signedTestLockfile(t)

	if err := trust.SignLockFile(ctx, lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("first SignLockFile: %v", err)
	}
	firstBundle := lf.Bundle
	// Re-sign: the payload (which excludes Bundle/RekorEntry) is unchanged, so the
	// new signature is over the same bytes; it must still verify.
	if err := trust.SignLockFile(ctx, lf, &trust.FakeSigner{NextLogIndex: 5}); err != nil {
		t.Fatalf("re-sign: %v", err)
	}
	if lf.Bundle == firstBundle {
		t.Fatal("re-signing did not change the bundle; the fake did not advance")
	}
	if err := trust.VerifyLockFile(ctx, lf, &trust.FakeVerifier{}); err != nil {
		t.Errorf("re-signed lockfile does not verify: %v", err)
	}
}
