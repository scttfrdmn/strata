package spec

import "testing"

// The R7-refuting exclusions named in environment_id_r7_fuzz_test.go's stated
// domain — same environment, different identity — each asserted to still
// reproduce.
//
// The stated domain has a second kind of exclusion which is NOT controlled here,
// and the gap is structural rather than an oversight: the opposite-reason
// exclusions (Defaults #118, ProfileName and RekorEntry #120, the layer
// manifest's Name/Version/InstallLayout #122) are cases where the environment
// differs and the identity does not. Witnessing one means calling
// overlay.ConfigureEnvironment and comparing the assembled roots, and
// internal/overlay imports spec, so a control for them cannot live in this
// package. It has to live in internal/overlay; #120 and #122 carry that as a
// task. Until it does, this file controls the four R7-refuting exclusions and
// every opposite-reason exclusion rests on its issue alone.
//
// These tests fail when a defect is FIXED. That is the intent. An exclusion list
// is a claim about the present, and the way such a list rots is that someone
// repairs the code and the list keeps quietly narrowing the property for years
// afterwards. Here the list cannot outlive the defects: fix #95 and
// TestR7Exclusion_ReorderPackageSets fails, which is the instruction to delete
// the exclusion and move the transformation into liveTransforms().
//
// They are also the control for the fuzz target's machinery, which is why they
// use the same clone-and-compare path rather than a private one. If clone()
// returned an alias, or EnvironmentID() returned a constant, or the comparison
// were inverted, FuzzR7NoSpuriousDistinctions would pass on everything and look
// exactly as it does now. These tests are what makes that distinguishable:
// a harness that cannot see a changed identity fails here.

// r7Fixture is a frozen lockfile with one layer and one package set, built
// explicitly rather than drawn from a stream so each control below is readable
// and deterministic.
func r7Fixture() *LockFile {
	return &LockFile{
		ProfileName:   "fixture",
		StrataVersion: "v0.22.0",
		Base:          ResolvedBase{DeclaredOS: "al2023", AMISHA256: "aaaaaaaa"},
		Layers: []ResolvedLayer{
			{LayerManifest: LayerManifest{Name: "python", Version: "3.11.9", SHA256: "bbbbbbbb"}, MountOrder: 1},
		},
		Packages: []ResolvedPackageSet{
			{Manager: "pip", Packages: []ResolvedPackageEntry{
				{Name: "numpy", Version: "2.1.0", SHA256: "cccccccc"},
				{Name: "scipy", Version: "1.14.0", SHA256: "dddddddd"},
			}},
			{Manager: "conda", Packages: []ResolvedPackageEntry{
				{Name: "samtools", Version: "1.21", SHA256: "eeeeeeee"},
			}},
		},
		OnReady: []string{"echo one", "echo two"},
	}
}

// assertStillSpurious asserts that mutate changes the EnvironmentID across a
// transformation the named issue claims preserves the assembled environment.
//
// The name is from when this file held four controls, each asserting a live R7
// counterexample that would expire when its defect was fixed. Both remaining
// callers are #95, whose environment-preserving premise the repo contradicts
// elsewhere (see the file comment), so what these assertions buy is a regression
// pin plus the fuzz target's machinery control — not an exclusion waiting to
// expire. The failure message reflects that; whether #95's pair are R7
// counterexamples at all is recorded on #95.
func assertStillSpurious(t *testing.T, issue, why string, mutate func(l *LockFile)) {
	t.Helper()

	original := r7Fixture()
	if !original.IsFrozen() {
		t.Fatal("fixture is not frozen, so EnvironmentID would be empty for both sides")
	}
	before := original.EnvironmentID()
	if before == "" {
		t.Fatal("fixture hashed to the empty string")
	}

	mutated := clone(original)
	mutate(mutated)

	if original.EnvironmentID() != before {
		t.Fatalf("the mutation reached the original lockfile; clone() is not deep enough " +
			"and the comparison below would be against a moving target")
	}

	after := mutated.EnvironmentID()
	if after == before {
		t.Errorf("%s: the identity no longer distinguishes these two lockfiles.\n"+
			"  the transformation, and what the issue claims about it: %s\n"+
			"  This is a REGRESSION. It is NOT an instruction to move the "+
			"transformation into liveTransforms():\n"+
			"    - package order is content. internal/agent/package_installer.go:37 "+
			"iterates the sets; :99 and :113 run one install command per entry, so a "+
			"later entry can change what an earlier one installed.\n"+
			"    - TestEnvironmentID_PackageOrderIsContent "+
			"(environment_id_scope_test.go:839) pins that behaviour and should be "+
			"failing beside this test. If it is passing, this test is the wrong one.\n"+
			"    - #95 prescribes sorting both dimensions of Packages. That "+
			"prescription is refuted on the issue for the reason above: it would give "+
			"two different install sequences one identity.\n"+
			"  Repair envHashInput, or revert the sort. Do not widen R7 over package "+
			"order, and do not delete this control — it is also what proves the fuzz "+
			"target can see a changed identity at all.\n"+
			"  id: %s", issue, why, before)
	}
}

func TestR7Exclusion_ReorderPackageSets(t *testing.T) {
	assertStillSpurious(t, "#95",
		"#95 claims the order two package sets are listed in does not change which "+
			"packages are installed, since every version is exactly pinned. That claim "+
			"is contradicted by internal/agent/package_installer.go:37,99,113 and the "+
			"contradiction is recorded on the issue; what is asserted here is only that "+
			"the identity distinguishes the two orders",
		func(l *LockFile) {
			l.Packages[0], l.Packages[1] = l.Packages[1], l.Packages[0]
		})
}

func TestR7Exclusion_ReorderPackageEntries(t *testing.T) {
	assertStillSpurious(t, "#95",
		"#95 claims a package set is a set, so listing numpy before scipy installs the "+
			"same two exactly-pinned packages as the reverse. One pip command runs per "+
			"entry, in order (internal/agent/package_installer.go:99), so the claim does "+
			"not hold; what is asserted here is only that the identity distinguishes the "+
			"two orders",
		func(l *LockFile) {
			p := l.Packages[0].Packages
			p[0], p[1] = p[1], p[0]
		})
}

// TestR7ExclusionNilEmptyIsNotAboutTheOuterSlice separates the defect from a
// neighbour it would otherwise be conflated with. The outer Packages field does
// carry omitempty, so nil and empty are both omitted there and the identity is
// unaffected. #117 is about the inner slice only, and stating the boundary keeps
// the register row from claiming more than the code does.
func TestR7ExclusionNilEmptyIsNotAboutTheOuterSlice(t *testing.T) {
	base := ResolvedBase{AMISHA256: "aaaaaaaa"}
	layers := []ResolvedLayer{
		{LayerManifest: LayerManifest{SHA256: "bbbbbbbb"}, MountOrder: 1},
	}

	nilOuter := &LockFile{Base: base, Layers: layers, Packages: nil}
	emptyOuter := &LockFile{Base: base, Layers: layers, Packages: []ResolvedPackageSet{}}

	if got, want := emptyOuter.EnvironmentID(), nilOuter.EnvironmentID(); got != want {
		t.Errorf("outer Packages nil vs empty changed the identity: %s != %s\n"+
			"If this fails, #117 is wider than its register row claims and the row "+
			"must be corrected to cover the outer slice too.", got, want)
	}
}
