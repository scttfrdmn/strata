// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package trust_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func testFormation(t *testing.T) *spec.Formation {
	t.Helper()
	return &spec.Formation{
		Name:    "genomics-python",
		Version: "2026.03",
		Layers: []spec.SoftwareRef{
			{Name: "python", Version: "3.13"},
			{Name: "samtools", Version: "1.23"},
		},
		Provides: []spec.Capability{
			{Name: "python", Version: "3.13"},
			{Name: "samtools", Version: "1.23"},
		},
	}
}

// TestSignAndVerifyFormation_RoundTrip: a freshly signed formation verifies, and
// signing records both the bundle and the transparency-log index — filling the
// pending-initial-build placeholders with real attestation (#237).
func TestSignAndVerifyFormation_RoundTrip(t *testing.T) {
	ctx := context.Background()
	f := testFormation(t)

	if err := trust.SignFormation(ctx, f, &trust.FakeSigner{}); err != nil {
		t.Fatalf("SignFormation: %v", err)
	}
	if f.Bundle == "" {
		t.Fatal("SignFormation did not record a bundle")
	}
	if f.RekorEntry == "" {
		t.Fatal("SignFormation did not record a transparency-log index")
	}
	if err := trust.VerifyFormation(ctx, f, &trust.FakeVerifier{}); err != nil {
		t.Fatalf("VerifyFormation of a freshly signed formation: %v", err)
	}
}

// TestVerifyFormation_RejectsTamper: after signing, any change to a field the
// signature covers must fail — a swapped/reordered layer ref, an added layer
// (a formation no maintainer attested), a changed version or provides set.
func TestVerifyFormation_RejectsTamper(t *testing.T) {
	ctx := context.Background()

	mutations := map[string]func(*spec.Formation){
		"swap a layer version": func(f *spec.Formation) { f.Layers[0].Version = "3.9" },
		"reorder the set":      func(f *spec.Formation) { f.Layers[0], f.Layers[1] = f.Layers[1], f.Layers[0] },
		"add a layer (mix-and-match)": func(f *spec.Formation) {
			f.Layers = append(f.Layers, spec.SoftwareRef{Name: "evil", Version: "1.0"})
		},
		"change the version":  func(f *spec.Formation) { f.Version = "2099.99" },
		"change the name":     func(f *spec.Formation) { f.Name = "attacker-formation" },
		"change the provides": func(f *spec.Formation) { f.Provides[0].Version = "9.9" },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := testFormation(t)
			if err := trust.SignFormation(ctx, f, &trust.FakeSigner{}); err != nil {
				t.Fatalf("SignFormation: %v", err)
			}
			mutate(f)
			if err := trust.VerifyFormation(ctx, f, &trust.FakeVerifier{}); err == nil {
				t.Errorf("VerifyFormation accepted a formation tampered after signing (%s)", name)
			}
		})
	}
}

// TestVerifyFormation_RejectsRekorEntryMismatch: RekorEntry is excluded from the
// signed payload, so the set signature still verifies if only RekorEntry is
// altered — but it is the formation's public transparency pointer, so
// verification must reject a value disagreeing with the signed bundle's index.
func TestVerifyFormation_RejectsRekorEntryMismatch(t *testing.T) {
	ctx := context.Background()
	f := testFormation(t)
	if err := trust.SignFormation(ctx, f, &trust.FakeSigner{NextLogIndex: 42}); err != nil {
		t.Fatalf("SignFormation: %v", err)
	}
	if f.RekorEntry != "42" {
		t.Fatalf("SignFormation recorded rekor_entry %q, want 42", f.RekorEntry)
	}

	f.RekorEntry = "999999" // falsify only the transparency pointer

	if err := trust.VerifyFormation(ctx, f, &trust.FakeVerifier{}); err == nil {
		t.Fatal("VerifyFormation accepted a formation whose rekor_entry does not match its bundle")
	} else if !strings.Contains(err.Error(), "rekor_entry") {
		t.Errorf("error does not name the rekor_entry mismatch: %v", err)
	}
}

// TestVerifyFormation_Unsigned: a formation with no bundle is refused — the
// pending-initial-build placeholder that has no real attestation does not pass.
func TestVerifyFormation_Unsigned(t *testing.T) {
	err := trust.VerifyFormation(context.Background(), testFormation(t), &trust.FakeVerifier{})
	if err == nil {
		t.Fatal("VerifyFormation accepted an unsigned formation")
	}
	if !strings.Contains(err.Error(), "not signed") {
		t.Errorf("error does not name the missing signature: %v", err)
	}
}

// TestSignFormation_SignedByIsNotSelfCovered: SignedBy is excluded from the
// signed payload (it is provenance the caller records, like a layer manifest's),
// so setting or changing it after signing does not break verification. This is
// the non-vacuity check that the elision is real — and that a verifier does not
// derive trust from the human-readable key ref.
func TestSignFormation_SignedByIsNotSelfCovered(t *testing.T) {
	ctx := context.Background()
	f := testFormation(t)
	if err := trust.SignFormation(ctx, f, &trust.FakeSigner{}); err != nil {
		t.Fatalf("SignFormation: %v", err)
	}
	f.SignedBy = "awskms:///alias/strata-signing-key" // recorded after signing
	if err := trust.VerifyFormation(ctx, f, &trust.FakeVerifier{}); err != nil {
		t.Errorf("setting SignedBy after signing broke verification: %v", err)
	}
}
