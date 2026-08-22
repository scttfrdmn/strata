package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// These tests re-derive what `strata run` actually announces about `packages:`
// entries, against today's code. The occasion is #137: the §4 register row for
// #48 was marked `Discharged: Yes — closed completed`, and §2.1 rule 11 says a
// closed issue is a fact about the tracker, not evidence about the tree.
//
// The re-derivation found the row and the code answering two different
// questions. #48 asked for a warning that the packages *will not be installed*
// by `strata run` (they are installed by strata-agent at instance boot); that is
// what cmd/strata/run.go:92-105 emits, and it is what these tests pin. The
// register row says `strata run` "did not warn that `packages:` entries are
// unattested" — a claim about the attestation chain, which no warning on either
// route makes. So these tests are the evidence for the *installation-visibility*
// claim and deliberately not for the row's wording; the row stays undischarged
// and the wording defect is filed separately, because rewording a counterexample
// in the same change that marks it discharged is narrowing-until-satisfied.
//
// Bound: chosen/implementation. Two package-set shapes on the `strata run` route
// only. Nothing here says anything about the strata-agent route, which installs
// these entries (internal/agent/package_installer.go) and warns about nothing.

// runPackagesFixture writes a lockfile carrying pkgs and returns its path plus a
// cache dir. RekorEntry is left empty on purpose: step 2 of runRun then prints
// "no Rekor entry", which is the marker the tests use to prove execution reached
// step 3 at all. Without that, a runRun that failed early would make the silence
// control pass by never running.
func runPackagesFixture(t *testing.T, pkgs []spec.ResolvedPackageSet) (lockPath, cacheDir string) {
	t.Helper()
	dir := t.TempDir()
	lf := spec.LockFile{
		ProfileName: "run-packages-fixture",
		Packages:    pkgs,
	}
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatalf("marshal lockfile: %v", err)
	}
	lockPath = filepath.Join(dir, "lock.yaml")
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}
	return lockPath, filepath.Join(dir, "cache")
}

// stderrOfRun drives runRun and returns everything it wrote to os.Stderr. The
// returned error is discarded: the fixture has no layers, so the run fails at
// the mount, well after the warnings under test. A failure *before* step 2 shows
// up as a missing "no Rekor entry" marker in every assertion below rather than
// as a pass.
func stderrOfRun(t *testing.T, lockPath, cacheDir string) string {
	t.Helper()
	return captureStderr(t, func() {
		_ = runRun(context.Background(), lockPath, []string{"/bin/true"}, true, cacheDir, "", nil)
	})
}

const reachedStep2 = "no Rekor entry"

// TestRunRun_AnnouncesPackagesItWillNotInstall is the evidence for what #48
// actually asked for.
//
// The first case carries three entries across two sets, which is the point of
// the shape: an implementation that counted package *sets* would print 2 and
// fail here. A one-set-one-entry fixture cannot tell the two counts apart.
func TestRunRun_AnnouncesPackagesItWillNotInstall(t *testing.T) {
	for _, tc := range []struct {
		name string
		pkgs []spec.ResolvedPackageSet
		want string
	}{
		{
			name: "three entries across two sets are counted as entries",
			pkgs: []spec.ResolvedPackageSet{
				{Manager: spec.PackageManagerPip, Packages: []spec.ResolvedPackageEntry{
					{Name: "numpy", Version: "1.26.4"},
					{Name: "scipy", Version: "1.14.1"},
				}},
				{Manager: spec.PackageManagerCRAN, Packages: []spec.ResolvedPackageEntry{
					{Name: "ggplot2", Version: "3.5.1"},
				}},
			},
			want: "3 package entries",
		},
		{
			name: "one entry is singular",
			pkgs: []spec.ResolvedPackageSet{
				{Manager: spec.PackageManagerConda, Packages: []spec.ResolvedPackageEntry{
					{Name: "mamba", Version: "1.5.8"},
				}},
			},
			want: "1 package entry",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lockPath, cacheDir := runPackagesFixture(t, tc.pkgs)
			got := stderrOfRun(t, lockPath, cacheDir)

			if !strings.Contains(got, reachedStep2) {
				t.Fatalf("runRun did not reach the warning steps (no %q on stderr); stderr was: %q",
					reachedStep2, got)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("stderr does not contain %q; stderr was: %q", tc.want, got)
			}
			// The count alone is not the warning #48 asked for. What makes it
			// actionable is naming who installs them and that this mount will
			// not.
			for _, want := range []string{"strata-agent", "will not include these packages"} {
				if !strings.Contains(got, want) {
					t.Errorf("stderr does not contain %q; stderr was: %q", want, got)
				}
			}
		})
	}
}

// TestRunRun_IsSilentWhenTheLockfileHasNoPackages is the control that gives the
// test above content: an implementation warning unconditionally would pass it.
func TestRunRun_IsSilentWhenTheLockfileHasNoPackages(t *testing.T) {
	lockPath, cacheDir := runPackagesFixture(t, nil)
	got := stderrOfRun(t, lockPath, cacheDir)

	// The premise, asserted before the absence is read: silence from a run that
	// never started is not evidence about the packages branch.
	if !strings.Contains(got, reachedStep2) {
		t.Fatalf("runRun did not reach the warning steps (no %q on stderr); stderr was: %q",
			reachedStep2, got)
	}
	if strings.Contains(got, "package entr") {
		t.Errorf("runRun warned about package entries for a lockfile with none; stderr was: %q", got)
	}
}

// TestRunRun_SaysNothingAboutPackageAttestation records the divergence the #137
// re-derivation turned up, as a test rather than as a note: the shipped warning
// is about installation, and nothing on this route tells the operator that
// `packages:` entries sit outside the layer attestation chain — they are fetched
// from PyPI/conda/CRAN by the agent, with no bundle and no Rekor entry.
//
// It asserts today's behaviour, so implementing that warning fails here and
// forces the register row to be revisited on purpose. That is the only mechanism
// that stops the row's `No` from outliving its reason.
func TestRunRun_SaysNothingAboutPackageAttestation(t *testing.T) {
	lockPath, cacheDir := runPackagesFixture(t, []spec.ResolvedPackageSet{
		{Manager: spec.PackageManagerPip, Packages: []spec.ResolvedPackageEntry{
			{Name: "numpy", Version: "1.26.4"},
		}},
	})
	got := stderrOfRun(t, lockPath, cacheDir)

	if !strings.Contains(got, reachedStep2) {
		t.Fatalf("runRun did not reach the warning steps (no %q on stderr); stderr was: %q",
			reachedStep2, got)
	}
	// Lower-cased once; the words are what matter, not their casing.
	lower := strings.ToLower(got)
	for _, unwanted := range []string{"unattested", "not attested", "attestation"} {
		if strings.Contains(lower, unwanted) {
			t.Errorf("stderr now mentions %q, so `strata run` does say something about "+
				"package attestation and the #48 register row must be revisited (#137); stderr was: %q",
				unwanted, got)
		}
	}
}
