package spec

import "testing"

// The exclusions named in environment_id_r7_fuzz_test.go's stated domain, each
// asserted to still reproduce.
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

// assertStillSpurious asserts that mutate changes the EnvironmentID even though
// the assembled environment is unchanged — i.e. that the named R7 counterexample
// still reproduces.
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
		t.Errorf("%s appears to be FIXED: the identity no longer distinguishes these "+
			"lockfiles.\n"+
			"  the environment was unchanged because: %s\n"+
			"  This test failing is the instruction to act, not a regression:\n"+
			"    1. move this transformation into liveTransforms() in "+
			"environment_id_r7_fuzz_test.go\n"+
			"    2. delete this control and its entry from that file's stated domain\n"+
			"    3. update the R7 row and %s's register row in PROPERTIES.md\n"+
			"  id: %s", issue, why, issue, before)
	}
}

func TestR7Exclusion_ReorderPackageSets(t *testing.T) {
	assertStillSpurious(t, "#95",
		"the order two package sets are listed in does not change which packages are "+
			"installed; every version is exactly pinned",
		func(l *LockFile) {
			l.Packages[0], l.Packages[1] = l.Packages[1], l.Packages[0]
		})
}

func TestR7Exclusion_ReorderPackageEntries(t *testing.T) {
	assertStillSpurious(t, "#95",
		"a package set is a set; listing numpy before scipy installs the same two "+
			"exactly-pinned packages as the reverse",
		func(l *LockFile) {
			p := l.Packages[0].Packages
			p[0], p[1] = p[1], p[0]
		})
}

func TestR7Exclusion_MutateOnReady(t *testing.T) {
	assertStillSpurious(t, "#69",
		"on_ready is hashed into the identity and executed by nothing — declared "+
			"spec/lockfile.go:39-40, copied internal/resolver/stages.go:434, no executor "+
			"in non-test code — so changing it cannot change what is assembled",
		func(l *LockFile) {
			l.OnReady = []string{"echo something entirely different"}
		})
}

func TestR7Exclusion_MutatePackageDigest(t *testing.T) {
	assertStillSpurious(t, "#98",
		"the installer resolves name@version from upstream and ignores the recorded "+
			"sha256, so two lockfiles differing only in that digest install identical bytes",
		func(l *LockFile) {
			l.Packages[0].Packages[0].SHA256 = "ffffffff"
		})
}

func TestR7Exclusion_NilVersusEmptyInnerPackages(t *testing.T) {
	assertStillSpurious(t, "#117",
		"a package set with no entries installs nothing whether the slice is nil or "+
			"empty; ResolvedPackageSet.Packages carries json:\"packages\" without "+
			"omitempty (spec/packages.go:49), so one marshals as null and the other as []",
		func(l *LockFile) {
			l.Packages = []ResolvedPackageSet{{Manager: "pip", Packages: nil}}
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
