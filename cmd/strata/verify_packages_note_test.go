// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestPackagesUnattestedNote is the evidence for the strata verify half of #139:
// a lockfile with package entries produces a disclosure that names the count and
// says plainly they are outside the attestation chain; a lockfile without any
// produces nothing (so verify stays quiet when there is nothing to disclose).
func TestPackagesUnattestedNote(t *testing.T) {
	if got := packagesUnattestedNote(&spec.LockFile{}); got != "" {
		t.Errorf("no packages should produce no note; got %q", got)
	}

	lf := &spec.LockFile{Packages: []spec.ResolvedPackageSet{
		{Manager: spec.PackageManagerPip, Packages: []spec.ResolvedPackageEntry{{Name: "numpy", Version: "1.26.4"}}},
	}}
	got := packagesUnattestedNote(lf)
	if !strings.Contains(got, "1 package entry") {
		t.Errorf("single entry should be singular; got %q", got)
	}
	for _, want := range []string{"attestation chain", "Rekor", "unattested", "--packages"} {
		if !strings.Contains(got, want) {
			t.Errorf("note missing %q: %q", want, got)
		}
	}
}
