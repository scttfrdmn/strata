package resolver_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_IgnoresInstanceAndStorage pins the advisory status of
// Profile.Instance and Profile.Storage (#102): they are recorded intent for
// downstream launch tooling, not consumed by resolution. LockFile carries
// neither field, so the check is that setting them changes nothing in the
// resolved environment — same identity, same layers. This is what makes their
// documentation as "advisory, not yet consumed" a tested fact rather than a
// claim, discharging the X1 defect (a *stated* unacted field is not a silent one)
// and the R3 one (their absence from the lockfile is by design).
func TestResolve_IgnoresInstanceAndStorage(t *testing.T) {
	store := registry.NewMemoryStore()
	store.AddLayer(signedLayer("python", "3.11.9", "linux-gnu-2.34",
		[]spec.Capability{{Name: "python", Version: "3.11.9"}}, nil))
	r := newResolver(t, store, nil)

	plain := testProfile(softwareRef("python", "3.11.9"))
	withProvisioning := testProfile(softwareRef("python", "3.11.9"))
	withProvisioning.Instance = spec.InstanceConfig{Type: "r7i.2xlarge", Spot: true, Placement: "cluster"}
	withProvisioning.Storage = []spec.StorageMount{{Type: "s3", Bucket: "b", Mount: "/data", ReadOnly: true}}

	lf1, err := r.Resolve(context.Background(), plain)
	if err != nil {
		t.Fatalf("resolve plain: %v", err)
	}
	lf2, err := r.Resolve(context.Background(), withProvisioning)
	if err != nil {
		t.Fatalf("resolve with instance/storage: %v", err)
	}

	if id := lf1.EnvironmentID(); id == "" {
		t.Fatal("resolved lockfile is not frozen; the identity comparison would be vacuous")
	}
	if lf1.EnvironmentID() != lf2.EnvironmentID() {
		t.Errorf("Instance/Storage changed the environment identity (#102): %s != %s",
			lf1.EnvironmentID(), lf2.EnvironmentID())
	}
	if len(lf1.Layers) != len(lf2.Layers) {
		t.Fatalf("Instance/Storage changed the layer count: %d vs %d", len(lf1.Layers), len(lf2.Layers))
	}
	for i := range lf1.Layers {
		if lf1.Layers[i].ID != lf2.Layers[i].ID || lf1.Layers[i].MountOrder != lf2.Layers[i].MountOrder {
			t.Errorf("layer %d differs with Instance/Storage set: %+v vs %+v",
				i, lf1.Layers[i], lf2.Layers[i])
		}
	}
}
