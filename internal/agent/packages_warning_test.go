// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestUnattestedPackagesWarning is the message half of the agent's #139
// disclosure: it counts entries across sets and states they are outside the
// attestation chain, and is empty when there are no packages.
func TestUnattestedPackagesWarning(t *testing.T) {
	if got := unattestedPackagesWarning(nil); got != "" {
		t.Errorf("no packages should produce no warning; got %q", got)
	}

	// Three entries across two sets — an implementation counting sets would say 2.
	pkgs := []spec.ResolvedPackageSet{
		{Manager: spec.PackageManagerPip, Packages: []spec.ResolvedPackageEntry{{Name: "numpy"}, {Name: "scipy"}}},
		{Manager: spec.PackageManagerCRAN, Packages: []spec.ResolvedPackageEntry{{Name: "ggplot2"}}},
	}
	got := unattestedPackagesWarning(pkgs)
	if !strings.Contains(got, "3 package entries") {
		t.Errorf("expected a count of 3 entries; got %q", got)
	}
	for _, want := range []string{"attestation chain", "Rekor", "unattested"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning missing %q: %q", want, got)
		}
	}
}

// TestAgentWarn_WritesToWarnings: warn goes to the configured writer, and a nil
// writer is a silent no-op (not a panic) — the agent must boot with no Warnings
// set.
func TestAgentWarn_WritesToWarnings(t *testing.T) {
	var buf bytes.Buffer
	(&Agent{cfg: Config{Warnings: &buf}}).warn("installing %d", 3)
	if !strings.Contains(buf.String(), "warning: installing 3") {
		t.Errorf("warn output: %q", buf.String())
	}
	// nil Warnings must not panic.
	(&Agent{}).warn("ignored")
}
