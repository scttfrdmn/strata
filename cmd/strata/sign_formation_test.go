// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func formationFixture() *spec.Formation {
	return &spec.Formation{
		Name:     "genomics-python",
		Version:  "2026.03",
		Layers:   []spec.SoftwareRef{{Name: "python", Version: "3.13"}, {Name: "samtools", Version: "1.23"}},
		Provides: []spec.Capability{{Name: "python", Version: "3.13"}},
		// The placeholders the shipped formations carry today.
		RekorEntry: "pending-initial-build",
		Bundle:     "pending-initial-build",
	}
}

// TestRunSignFormation_SignsAndRecords exercises the command wiring through the
// signFormation seam with a fake signer (no AWS/cosign): the placeholder
// rekor_entry/bundle are replaced with a real bundle + Rekor index, SignedBy is
// recorded, and the result verifies (#237).
func TestRunSignFormation_SignsAndRecords(t *testing.T) {
	orig := signFormation
	t.Cleanup(func() { signFormation = orig })
	signFormation = func(ctx context.Context, f *spec.Formation, _ string) error {
		return trust.SignFormation(ctx, f, &trust.FakeSigner{})
	}

	p := filepath.Join(t.TempDir(), "genomics-python@2026.03.yaml")
	if err := writeYAML(p, formationFixture()); err != nil {
		t.Fatal(err)
	}
	if err := runSignFormation(p, defaultSigningKey); err != nil {
		t.Fatalf("runSignFormation: %v", err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("re-reading signed formation: %v", err)
	}
	var signed spec.Formation
	if err := yaml.Unmarshal(data, &signed); err != nil {
		t.Fatalf("parsing signed formation: %v", err)
	}
	if signed.Bundle == "" || signed.Bundle == "pending-initial-build" {
		t.Errorf("placeholder bundle was not replaced: %q", signed.Bundle)
	}
	if signed.RekorEntry == "" || signed.RekorEntry == "pending-initial-build" {
		t.Errorf("placeholder rekor_entry was not replaced: %q", signed.RekorEntry)
	}
	if signed.SignedBy != defaultSigningKey {
		t.Errorf("SignedBy = %q, want the signing key ref", signed.SignedBy)
	}
	if err := trust.VerifyFormation(context.Background(), &signed, &trust.FakeVerifier{}); err != nil {
		t.Errorf("signed formation does not verify: %v", err)
	}
}

// TestRunSignFormation_RejectsNonFormation: a file with no name/version or no
// layers is not a formation and is refused before any key is touched.
func TestRunSignFormation_RejectsNonFormation(t *testing.T) {
	cases := map[string]*spec.Formation{
		"no name/version": {Layers: []spec.SoftwareRef{{Name: "x", Version: "1"}}},
		"no layers":       {Name: "empty", Version: "1.0"},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "f.yaml")
			if err := writeYAML(p, f); err != nil {
				t.Fatal(err)
			}
			if err := runSignFormation(p, "unused"); err == nil {
				t.Fatal("runSignFormation accepted a non-formation file")
			}
		})
	}
}
