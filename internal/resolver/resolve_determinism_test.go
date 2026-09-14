package resolver_test

import (
	"context"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// determinismStore builds three mutually-independent layers (no requirement
// edges between them), so the only thing deciding their relative order in the
// lockfile is the resolver's content tie-break — the surface R2 is about.
func determinismStore() *registry.MemoryStore {
	s := registry.NewMemoryStore()
	s.AddLayer(signedLayer("alpha", "1.0.0", "linux-gnu-2.34",
		[]spec.Capability{{Name: "alpha", Version: "1.0.0"}}, nil))
	s.AddLayer(signedLayer("bravo", "2.1.0", "linux-gnu-2.34",
		[]spec.Capability{{Name: "bravo", Version: "2.1.0"}}, nil))
	s.AddLayer(signedLayer("charlie", "0.9.3", "linux-gnu-2.34",
		[]spec.Capability{{Name: "charlie", Version: "0.9.3"}}, nil))
	return s
}

// serializeEliding marshals a lockfile to YAML after zeroing the fields R1 and
// R2 enumerate as elided: the two wall-clock timestamps — resolved_at and
// base.capabilities.probed_at, both recorded from the clock and both excluded
// from the identity by design (EnvironmentID and BaseCapabilities.ContentDigest
// respectively, spec/base_digest.go) — and, for the permutation case,
// profile_sha256, which records the byte order of the input profile. It zeroes
// the struct fields and re-marshals rather than filtering lines out of the
// output, so that renaming a field cannot silently make the elision match
// nothing (numbers-need-a-command: a filter must match the format of what it
// filters).
func serializeEliding(t *testing.T, lf *spec.LockFile, elideProfileSHA bool) string {
	t.Helper()
	clone := *lf
	clone.ResolvedAt = time.Time{}
	clone.Base.Capabilities.ProbedAt = time.Time{}
	if elideProfileSHA {
		clone.ProfileSHA256 = ""
	}
	b, err := yaml.Marshal(&clone)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func rawSerialize(t *testing.T, lf *spec.LockFile) string {
	t.Helper()
	b, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestResolve_IdenticalInputsAreByteIdenticalElidingResolvedAt is R1's direct
// instrument. Two resolutions of the same profile against the same registry and
// resolver version must produce lockfiles that are byte-identical once the two
// enumerated wall-clock fields — resolved_at and base.capabilities.probed_at —
// are elided; every other byte, including fields that do not participate in
// EnvironmentID, must match. This is the "full canonical serialisation with the
// wall-clock fields elided" instrument R1's register row names as the test that
// could refute it, which no test previously supplied (the old citation compared
// two derived hashes and left every other field free to differ). Writing this
// test is what found the second wall-clock field: R1 enumerated only
// resolved_at, and probed_at also varies between two probes of the same base.
func TestResolve_IdenticalInputsAreByteIdenticalElidingResolvedAt(t *testing.T) {
	store := determinismStore()
	profile := testProfile(
		softwareRef("alpha", "1.0.0"),
		softwareRef("bravo", "2.1.0"),
		softwareRef("charlie", "0.9.3"),
	)

	r := newResolver(t, store, nil)
	first, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Force resolved_at to differ so the elision below is non-vacuous: if the two
	// resolves happened to record the same wall-clock instant, the "differs
	// before eliding" check would pass for the wrong reason.
	time.Sleep(2 * time.Millisecond)
	second, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	// Non-triviality: three layers resolved, or "byte-identical" is a cheap claim
	// about an almost-empty document.
	if len(first.Layers) != 3 {
		t.Fatalf("fixture resolved %d layers, expected 3 — comparison would be near-vacuous", len(first.Layers))
	}

	// Non-vacuity: without eliding, the two lockfiles must differ, which proves
	// resolved_at actually varied. If they were already identical the elision
	// step would be asserting nothing.
	if rawSerialize(t, first) == rawSerialize(t, second) {
		t.Fatal("two resolves produced byte-identical lockfiles including the wall-clock fields — " +
			"the elision check below cannot then witness R1's exception is confined to them")
	}

	// R1: identical after eliding exactly the wall-clock fields.
	if got, want := serializeEliding(t, first, false), serializeEliding(t, second, false); got != want {
		t.Errorf("two resolutions of identical inputs differ in a field other than the wall-clock ones (R1):\n--- first\n%s\n--- second\n%s", got, want)
	}
}

// TestResolve_PermutingSoftwareIsByteIdenticalElidingInputRecordingFields is
// R2's direct instrument, and the stronger form the register's residual asked
// for: not just that mount_order and EnvironmentID are permutation-invariant
// (mountorder_permutation_test.go already holds that), but that the *entire*
// lockfile is, once the fields that do not describe the assembled environment
// are elided — the two wall-clock fields (resolved_at, probed_at) and
// profile_sha256, which hashes the profile bytes and so changes when the
// software: list is reordered. If any field that describes the assembled
// environment moved with the permutation, the elided serialisations would
// differ and this test would fail.
func TestResolve_PermutingSoftwareIsByteIdenticalElidingInputRecordingFields(t *testing.T) {
	a := softwareRef("alpha", "1.0.0")
	b := softwareRef("bravo", "2.1.0")
	c := softwareRef("charlie", "0.9.3")

	resolve := func(refs ...spec.SoftwareRef) *spec.LockFile {
		t.Helper()
		r := newResolver(t, determinismStore(), nil)
		lf, err := r.Resolve(context.Background(), testProfile(refs...))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		return lf
	}

	forward := resolve(a, b, c)
	reverse := resolve(c, b, a)

	if len(forward.Layers) != 3 {
		t.Fatalf("fixture resolved %d layers, expected 3", len(forward.Layers))
	}

	// Non-vacuity: the two orderings must produce lockfiles that differ before
	// eliding — profile_sha256 records the input byte order, so it must differ —
	// otherwise the elided-equality claim is empty.
	if rawSerialize(t, forward) == rawSerialize(t, reverse) {
		t.Fatal("permuted profiles produced byte-identical lockfiles including profile_sha256 — " +
			"the fixture is not exercising the input-order-recording fields R2 elides")
	}

	// R2: identical after eliding the input-recording fields {resolved_at, profile_sha256}.
	if got, want := serializeEliding(t, forward, true), serializeEliding(t, reverse, true); got != want {
		t.Errorf("permuting software: changed a field other than the input-recording ones (R2):\n--- forward\n%s\n--- reverse\n%s", got, want)
	}

	// The identity is the load-bearing consequence: same environment, same ID.
	if id := forward.EnvironmentID(); id == "" {
		t.Fatal("resolved lockfile is not frozen; the EnvironmentID equality below would be \"\" == \"\"")
	}
	if forward.EnvironmentID() != reverse.EnvironmentID() {
		t.Errorf("permuting software: changed EnvironmentID (R2):\n  forward %s\n  reverse %s",
			forward.EnvironmentID(), reverse.EnvironmentID())
	}
}
