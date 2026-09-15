package resolver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_OverlappingFormations is #208 fixed, and R2's instrument over the
// domain the v0.25.0 differential test could not reach. Two formations that share
// a layer now resolve to ONE instance of it, with provenance unioned, so:
//
//   - the shared squashfs is mounted once, not twice (correctness), and
//   - the full lockfile is permutation-invariant (R2) — the surviving layer's
//     satisfied_by/from_formation are the sorted union of both formations, the
//     same regardless of software: order.
//
// Before #208 this reproduced the refutation: python mounted twice, and the
// two instances' provenance swapped with input order.
func TestResolve_OverlappingFormations(t *testing.T) {
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

	// The shared python layer is now mounted once.
	var pythonFwd *spec.ResolvedLayer
	pythonCount := 0
	for i := range forward.Layers {
		if forward.Layers[i].Name == "python" {
			pythonCount++
			pythonFwd = &forward.Layers[i]
		}
	}
	if pythonCount != 1 {
		t.Fatalf("shared python layer mounted %d times, want 1 (#208 dedup)", pythonCount)
	}

	// Its provenance is the sorted union of both formations — deterministic.
	if !strings.Contains(pythonFwd.SatisfiedBy, "aformation@1.0") ||
		!strings.Contains(pythonFwd.SatisfiedBy, "bformation@1.0") {
		t.Errorf("deduped layer does not record both requesters: satisfied_by = %q", pythonFwd.SatisfiedBy)
	}
	if pythonFwd.FromFormation != "aformation@1.0, bformation@1.0" {
		t.Errorf("from_formation is not the sorted union: %q", pythonFwd.FromFormation)
	}

	// R2: the whole lockfile is permutation-invariant once the input/clock fields
	// are elided — the refutation this test replaced.
	if serializeEliding(t, forward, true) != serializeEliding(t, reverse, true) {
		t.Errorf("permuting overlapping formations changed the lockfile (R2):\n--- forward\n%s\n--- reverse\n%s",
			serializeEliding(t, forward, true), serializeEliding(t, reverse, true))
	}

	// And the identity is invariant and non-empty.
	if id := forward.EnvironmentID(); id == "" || id != reverse.EnvironmentID() {
		t.Errorf("EnvironmentID not stable across permutation: %q vs %q", id, reverse.EnvironmentID())
	}
}
