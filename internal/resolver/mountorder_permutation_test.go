package resolver_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_MountOrderIsPermutationInvariant is R2's direct instrument: three
// mutually-independent layers (no requirement edges between them) are placed by
// the topological sort's tie-break alone. Under the pre-#95 tie-break —
// sort.Ints(queue), i.e. layer index, i.e. `software:` declaration order —
// permuting the software list reordered them, changed every MountOrder, and
// changed the EnvironmentID. The content tie-break (name, version, ID) must make
// the mount order a function of what the layers are, not the order they were
// written down.
func TestResolve_MountOrderIsPermutationInvariant(t *testing.T) {
	newStore := func() *registry.MemoryStore {
		s := registry.NewMemoryStore()
		// nil requires ⇒ every layer has in-degree zero ⇒ all sit in the initial
		// ready set together, so the tie-break is the only thing ordering them.
		s.AddLayer(signedLayer("alpha", "1.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "alpha", Version: "1.0.0"}}, nil))
		s.AddLayer(signedLayer("bravo", "1.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "bravo", Version: "1.0.0"}}, nil))
		s.AddLayer(signedLayer("charlie", "1.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "charlie", Version: "1.0.0"}}, nil))
		return s
	}

	resolve := func(refs ...spec.SoftwareRef) *spec.LockFile {
		t.Helper()
		r := newResolver(t, newStore(), nil)
		lf, err := r.Resolve(context.Background(), testProfile(refs...))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		return lf
	}

	a := softwareRef("alpha", "1.0.0")
	b := softwareRef("bravo", "1.0.0")
	c := softwareRef("charlie", "1.0.0")

	forward := resolve(a, b, c)
	reverse := resolve(c, b, a)

	orderByName := func(lf *spec.LockFile) map[string]int {
		m := make(map[string]int, len(lf.Layers))
		for _, l := range lf.Layers {
			m[l.Name] = l.MountOrder
		}
		return m
	}
	fwd, rev := orderByName(forward), orderByName(reverse)

	// Non-vacuity: the three layers resolved and were assigned the distinct set
	// {1,2,3}. A vacuous run (all equal, or fewer layers) would make the equality
	// below pass for the wrong reason.
	if len(fwd) != 3 {
		t.Fatalf("expected 3 layers, got %d: %v", len(fwd), fwd)
	}
	seen := map[int]bool{}
	for _, o := range fwd {
		seen[o] = true
	}
	if !seen[1] || !seen[2] || !seen[3] {
		t.Fatalf("mount orders are not the distinct set {1,2,3}: %v", fwd)
	}

	if !reflect.DeepEqual(fwd, rev) {
		t.Errorf("permuting software: changed mount order (R2):\n  forward %v\n  reverse %v", fwd, rev)
	}

	// The identity is what the mount order feeds; a resolved lockfile carries the
	// base digest (#64) and layer digests, so it is frozen and has a non-empty ID.
	if id := forward.EnvironmentID(); id == "" {
		t.Fatal("resolved lockfile is not frozen; the EnvironmentID check below would be \"\" == \"\"")
	}
	if forward.EnvironmentID() != reverse.EnvironmentID() {
		t.Errorf("permuting software: changed EnvironmentID (R2/R7):\n  forward %s\n  reverse %s",
			forward.EnvironmentID(), reverse.EnvironmentID())
	}
}
