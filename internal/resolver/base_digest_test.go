package resolver_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/testregistry"
)

// TestResolveSetsBaseAMISHA256 is #64's outcome at the resolver level: stage 8
// now records the base's content digest, so a fully-built profile resolves to a
// lockfile IsFrozen() accepts — which is what strata freeze structurally could
// not reach before.
func TestResolveSetsBaseAMISHA256(t *testing.T) {
	r, _ := newFixtureResolver(t)
	profile := fixtureProfile(t, testregistry.ProfileMinimal)

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if lf.Base.AMISHA256 == "" {
		t.Fatal("resolved lockfile has empty Base.AMISHA256 — freeze would remain impossible (#64)")
	}
	if want := lf.Base.Capabilities.ContentDigest(); lf.Base.AMISHA256 != want {
		t.Errorf("Base.AMISHA256 = %q, want the capability digest %q", lf.Base.AMISHA256, want)
	}
	// The fixture's layers are all pinned, so with AMISHA256 populated the
	// lockfile is now freezable — the whole point of #64.
	if !lf.IsFrozen() {
		t.Error("a fully-built profile resolved to a lockfile IsFrozen() still rejects")
	}
}
