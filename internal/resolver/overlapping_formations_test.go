package resolver_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_OverlappingFormations_R2Counterexample pins the #208 defect and is
// R2's executed counterexample over a domain the v0.25.0 differential test did
// not reach (it used only standalone, distinct layers). Two formations that
// share a layer:
//
//   - mount that layer twice (a correctness defect — the same squashfs appears
//     in the stack twice), and
//   - attach satisfied_by/from_formation to the duplicates in software: order,
//     which refutes R2 (those fields are not among its permitted elisions),
//     while EnvironmentID stays invariant (the hashed fields are identical).
//
// It is an exclusion control: it asserts today's behaviour, so #208's dedup fix
// reddens it and forces R2's register row to be re-derived rather than the
// refutation silently lapsing. If this test fails with "did #208 land", invert
// it (equal serialisations, one instance of the shared layer) and move R2 back
// to ENFORCED with the overlapping-formation domain covered.
func TestResolve_OverlappingFormations_R2Counterexample(t *testing.T) {
	newStore := func() *registry.MemoryStore {
		s := registry.NewMemoryStore()
		s.AddLayer(signedLayer("python", "3.11.9", "linux-gnu-2.34",
			[]spec.Capability{{Name: "python", Version: "3.11.9"}}, nil))
		s.AddLayer(signedLayer("notebook", "7.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "notebook", Version: "7.0.0"}}, nil))
		s.AddLayer(signedLayer("scipy", "1.12.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "scipy", Version: "1.12.0"}}, nil))
		s.AddFormation(&spec.Formation{Name: "aformation", Version: "1.0", Bundle: "s3://b/a", RekorEntry: "1",
			Layers: []spec.SoftwareRef{{Name: "python", Version: "3.11.9"}, {Name: "notebook", Version: "7.0.0"}}})
		s.AddFormation(&spec.Formation{Name: "bformation", Version: "1.0", Bundle: "s3://b/b", RekorEntry: "2",
			Layers: []spec.SoftwareRef{{Name: "python", Version: "3.11.9"}, {Name: "scipy", Version: "1.12.0"}}})
		return s
	}
	resolve := func(refs ...spec.SoftwareRef) *spec.LockFile {
		lf, err := newResolver(t, newStore(), nil).Resolve(context.Background(), testProfile(refs...))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		return lf
	}

	forward := resolve(formationRef("aformation@1.0"), formationRef("bformation@1.0"))
	reverse := resolve(formationRef("bformation@1.0"), formationRef("aformation@1.0"))

	// The correctness half: the shared python layer is mounted twice today.
	pythonCount := 0
	for _, l := range forward.Layers {
		if l.Name == "python" {
			pythonCount++
		}
	}
	if pythonCount != 2 {
		t.Fatalf("expected the pre-#208 double-mount (python x2), got python x%d — did #208 land? invert this test", pythonCount)
	}

	// The identity is invariant — the hashed fields (name/version/ID/SHA256) are
	// the same multiset — which is why R7's generator cannot see this.
	if forward.EnvironmentID() != reverse.EnvironmentID() {
		t.Errorf("EnvironmentID differs across permutation: %s vs %s", forward.EnvironmentID(), reverse.EnvironmentID())
	}

	// The R2 half: eliding the input/clock fields, the full serialisations still
	// differ — satisfied_by/from_formation follow software: order. When #208
	// dedups, these become equal and this assertion must be inverted.
	if serializeEliding(t, forward, true) == serializeEliding(t, reverse, true) {
		t.Fatal("overlapping-formation serialisations are now permutation-invariant — did #208 land? invert this test and re-derive R2")
	}
}
