package resolver_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// These tests are the offline end-to-end path: profile in, lockfile out, with no
// AWS credentials, no network, and no prior build. Before the testregistry
// fixture existed there was no way to reach stage 8 at all — the embedded recipe
// catalog carries no bundles, so every profile died in stage 7 (#54) — which
// meant no defect in the trust, freeze, or publish paths could carry an
// end-to-end regression test.
//
// The registry here is a real registry.LocalClient over real files, not a mock.
// A mocked resolver would pass whether or not resolution actually works.

func newFixtureResolver(t *testing.T) (*resolver.Resolver, string) {
	t.Helper()
	root, client := testregistry.New(t)
	probeClient, err := testregistry.Probe()
	if err != nil {
		t.Fatalf("fixture probe client: %v", err)
	}
	r, err := resolver.New(resolver.Config{
		Registry:      client,
		Probe:         probeClient,
		StrataVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("resolver.New: %v", err)
	}
	return r, root
}

func fixtureProfile(t *testing.T, name string) *spec.Profile {
	t.Helper()
	data, err := testregistry.ProfileBytes(name)
	if err != nil {
		t.Fatalf("%v", err)
	}
	p, err := spec.ParseProfileBytes(data)
	if err != nil {
		t.Fatalf("parsing fixture profile %s: %v", name, err)
	}
	return p
}

// TestOfflineResolveProducesLockfile is the regression test for #54: resolution
// reaches stage 8 offline and the lockfile it produces is complete enough to be
// re-parsed.
func TestOfflineResolveProducesLockfile(t *testing.T) {
	r, _ := newFixtureResolver(t)
	profile := fixtureProfile(t, testregistry.ProfileMinimal)

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if lf == nil {
		t.Fatal("Resolve returned a nil lockfile and a nil error")
	}
	if len(lf.Layers) != len(testregistry.LayerIDs) {
		t.Fatalf("lockfile has %d layers, want %d", len(lf.Layers), len(testregistry.LayerIDs))
	}

	for _, layer := range lf.Layers {
		if layer.SHA256 == "" {
			t.Errorf("%s: lockfile layer has no sha256", layer.ID)
		}
		if layer.RekorEntry == "" {
			t.Errorf("%s: lockfile layer has no rekor_entry", layer.ID)
		}
		path, ok := strings.CutPrefix(layer.Bundle, "file://")
		if !ok {
			t.Errorf("%s: bundle %q is not a file:// URI", layer.ID, layer.Bundle)
			continue
		}
		// The point of the fixture is that the bundle is a file that exists, not
		// a placeholder string that merely satisfies a non-empty check (#61).
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: bundle referenced by the lockfile does not exist: %v", layer.ID, err)
		}
	}

	// Round-trip through the spec parser: a lockfile that cannot be re-read is
	// not a usable artifact for the commands that consume one.
	data, err := spec.MarshalLockFile(lf)
	if err != nil {
		t.Fatalf("MarshalLockFile: %v", err)
	}
	if _, err := spec.ParseLockFileBytes(data); err != nil {
		t.Fatalf("ParseLockFileBytes on freshly resolved lockfile: %v", err)
	}
}

// TestOfflineResolveMountOrderFollowsDependencies is the discriminating half of
// the fixture: the profile lists jupyterlab before python, and jupyterlab
// requires python, so a resolver that preserved input order would produce the
// opposite order here. Asserting only "two layers resolved" would pass either way.
func TestOfflineResolveMountOrderFollowsDependencies(t *testing.T) {
	r, _ := newFixtureResolver(t)
	profile := fixtureProfile(t, testregistry.ProfileMinimal)

	if profile.Software[0].Name != "jupyterlab" {
		t.Fatalf("fixture profile no longer lists jupyterlab first (got %q) — this test's premise is gone",
			profile.Software[0].Name)
	}

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got := make([]string, len(lf.Layers))
	for i, layer := range lf.Layers {
		got[i] = layer.ID
		if layer.MountOrder != i+1 {
			t.Errorf("layer %d (%s): mount_order = %d, want %d", i, layer.ID, layer.MountOrder, i+1)
		}
	}
	for i, want := range testregistry.LayerIDs {
		if got[i] != want {
			t.Errorf("layer %d = %s, want %s (dependency order: python before jupyterlab)", i, got[i], want)
		}
	}
}

// TestOfflineResolveFormation puts stage 2 on the offline path: the same two
// layers reached through a formation ref, including the formation's own
// bundle and rekor_entry presence checks.
func TestOfflineResolveFormation(t *testing.T) {
	r, _ := newFixtureResolver(t)
	profile := fixtureProfile(t, testregistry.ProfileFormation)

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(lf.Layers) != len(testregistry.LayerIDs) {
		t.Fatalf("lockfile has %d layers, want %d", len(lf.Layers), len(testregistry.LayerIDs))
	}
	for _, layer := range lf.Layers {
		if layer.FromFormation != testregistry.FormationRef {
			t.Errorf("%s: from_formation = %q, want %q", layer.ID, layer.FromFormation, testregistry.FormationRef)
		}
	}
}

// TestOfflineResolveNoPartialLockfileOnFailure checks the property #54's
// evidence observed behaviourally — a failed resolution returns no lockfile at
// all — against a stage-3 failure this fixture can produce on demand.
func TestOfflineResolveNoPartialLockfileOnFailure(t *testing.T) {
	r, _ := newFixtureResolver(t)
	profile := fixtureProfile(t, testregistry.ProfileMinimal)
	profile.Software = append(profile.Software, spec.SoftwareRef{Name: "no-such-layer", Version: "1.0"})

	lf, err := r.Resolve(context.Background(), profile)
	if err == nil {
		t.Fatal("Resolve succeeded with an unresolvable software ref")
	}
	if lf != nil {
		t.Errorf("Resolve returned a partial lockfile alongside an error: %+v", lf)
	}
}
