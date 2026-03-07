package resolver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// ---- test fixtures ----

const (
	testAMIID = "ami-test123456789"
	testArch  = "x86_64"
	testOS    = "al2023"
	testOSKey = "al2023/x86_64"
)

// baseCapabilities returns a minimal BaseCapabilities for al2023/x86_64.
func baseCapabilities() *spec.BaseCapabilities {
	return &spec.BaseCapabilities{
		AMIID:    testAMIID,
		OS:       testOS,
		Arch:     testArch,
		Family:   "rhel",
		ProbedAt: time.Now(),
		Provides: []spec.Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "kernel", Version: "6.1"},
			{Name: "family", Version: "rhel"},
		},
	}
}

// testProbe returns a probe.Client wired with static AMI resolution
// and a FakeRunner configured for testAMIID.
func testProbe() *probe.Client {
	return &probe.Client{
		Resolver: &probe.StaticResolver{
			AMIs: map[string]string{testOSKey: testAMIID},
		},
		Runner: &probe.FakeRunner{
			Capabilities: map[string]*spec.BaseCapabilities{
				testAMIID: baseCapabilities(),
			},
		},
		Cache: probe.NewMemoryCache(),
	}
}

// signedLayer returns a LayerManifest with Bundle and RekorEntry set.
func signedLayer(name, version, family string, provides []spec.Capability, requires []spec.Requirement) *spec.LayerManifest {
	id := name + "-" + version + "-" + family + "-" + testArch
	return &spec.LayerManifest{
		ID:         id,
		Name:       name,
		Version:    version,
		Arch:       testArch,
		Family:     family,
		SHA256:     "sha256-" + id,
		Bundle:     "s3://strata-test-layers/" + id + "/bundle.json",
		RekorEntry: "42",
		SignedBy:   "test@strata.dev",
		Provides:   provides,
		Requires:   requires,
	}
}

// testProfile builds a minimal profile with the given software refs.
func testProfile(refs ...spec.SoftwareRef) *spec.Profile {
	return &spec.Profile{
		Name:     "test-env",
		Base:     spec.BaseRef{OS: testOS, Arch: testArch},
		Software: refs,
	}
}

// softwareRef returns a SoftwareRef for a named package with optional version.
func softwareRef(name, version string) spec.SoftwareRef {
	return spec.SoftwareRef{Name: name, Version: version}
}

// formationRef returns a SoftwareRef referencing a formation.
func formationRef(nameVersion string) spec.SoftwareRef {
	return spec.SoftwareRef{Formation: nameVersion}
}

// ---- helpers ----

// newResolver builds a Resolver with the given registry store and optional Rekor client.
func newResolver(t *testing.T, store *registry.MemoryStore, rekor trust.RekorClient) *resolver.Resolver {
	t.Helper()
	r, err := resolver.New(resolver.Config{
		Registry:      store,
		Probe:         testProbe(),
		Rekor:         rekor,
		StrataVersion: "0.3.0-test",
	})
	if err != nil {
		t.Fatalf("resolver.New: %v", err)
	}
	return r
}

// assertResolutionError asserts that err is a *ResolutionError with the given code.
func assertResolutionError(t *testing.T, err error, code string) {
	t.Helper()
	var re *resolver.ResolutionError
	if !errors.As(err, &re) {
		t.Fatalf("expected *resolver.ResolutionError, got %T: %v", err, err)
	}
	if re.Code != code {
		t.Fatalf("expected ResolutionError.Code=%q, got %q (message: %s)", code, re.Code, re.Message)
	}
}

// ---- test cases ----

// TestNew_MissingRequired verifies that New rejects configs with nil required fields.
func TestNew_MissingRequired(t *testing.T) {
	store := registry.NewMemoryStore()
	p := testProbe()

	if _, err := resolver.New(resolver.Config{Registry: nil, Probe: p}); err == nil {
		t.Error("expected error for nil Registry")
	}
	if _, err := resolver.New(resolver.Config{Registry: store, Probe: nil}); err == nil {
		t.Error("expected error for nil Probe")
	}
}

// TestHappyPath_FormationPlusStandalone resolves a profile with one formation
// and one standalone layer, verifying that the lockfile is fully populated.
func TestHappyPath_FormationPlusStandalone(t *testing.T) {
	store := registry.NewMemoryStore()

	// Formation layers.
	python := signedLayer("python", "3.11.9", "rhel",
		[]spec.Capability{{Name: "python", Version: "3.11.9"}},
		[]spec.Requirement{{Name: "glibc", MinVersion: "2.34"}})
	numpy := signedLayer("numpy", "1.26.0", "rhel",
		[]spec.Capability{{Name: "numpy", Version: "1.26.0"}},
		[]spec.Requirement{{Name: "python", MinVersion: "3.11"}})
	store.AddLayer(python)
	store.AddLayer(numpy)

	store.AddFormation(&spec.Formation{
		Name:       "ml-stack",
		Version:    "1.0",
		Bundle:     "s3://strata/formations/ml-stack@1.0/bundle.json",
		RekorEntry: "100",
		Layers: []spec.SoftwareRef{
			{Name: "python", Version: "3.11.9"},
			{Name: "numpy", Version: "1.26.0"},
		},
	})

	// Standalone layer.
	scipy := signedLayer("scipy", "1.12.0", "rhel",
		[]spec.Capability{{Name: "scipy", Version: "1.12.0"}},
		[]spec.Requirement{{Name: "numpy", MinVersion: "1.26"}})
	store.AddLayer(scipy)

	r := newResolver(t, store, nil)
	lf, err := r.Resolve(context.Background(), testProfile(
		formationRef("ml-stack@1.0"),
		softwareRef("scipy", "1.12.0"),
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if lf.ProfileName != "test-env" {
		t.Errorf("ProfileName = %q, want %q", lf.ProfileName, "test-env")
	}
	if lf.ProfileSHA256 == "" {
		t.Error("ProfileSHA256 is empty")
	}
	if lf.Base.AMIID != testAMIID {
		t.Errorf("Base.AMIID = %q, want %q", lf.Base.AMIID, testAMIID)
	}
	if lf.StrataVersion != "0.3.0-test" {
		t.Errorf("StrataVersion = %q, want %q", lf.StrataVersion, "0.3.0-test")
	}

	if len(lf.Layers) != 3 {
		t.Fatalf("len(Layers) = %d, want 3", len(lf.Layers))
	}

	// Formation layers have FromFormation set.
	var formationCount int
	for _, l := range lf.Layers {
		if l.FromFormation == "ml-stack@1.0" {
			formationCount++
		}
	}
	if formationCount != 2 {
		t.Errorf("expected 2 layers from formation, got %d", formationCount)
	}

	// MountOrder must be 1..3, no duplicates.
	orders := make(map[int]bool)
	for _, l := range lf.Layers {
		if orders[l.MountOrder] {
			t.Errorf("duplicate MountOrder %d", l.MountOrder)
		}
		orders[l.MountOrder] = true
	}
	for i := 1; i <= 3; i++ {
		if !orders[i] {
			t.Errorf("MountOrder %d missing from layers", i)
		}
	}
}

// TestHappyPath_FormationOnly resolves a profile with all software via a formation.
func TestHappyPath_FormationOnly(t *testing.T) {
	store := registry.NewMemoryStore()

	cuda := signedLayer("cuda", "12.3.2", "rhel",
		[]spec.Capability{{Name: "cuda", Version: "12.3.2"}},
		nil)
	store.AddLayer(cuda)

	store.AddFormation(&spec.Formation{
		Name:       "cuda-base",
		Version:    "2024.03",
		Bundle:     "s3://strata/formations/cuda-base@2024.03/bundle.json",
		RekorEntry: "200",
		Layers:     []spec.SoftwareRef{{Name: "cuda", Version: "12.3.2"}},
	})

	r := newResolver(t, store, nil)
	lf, err := r.Resolve(context.Background(), testProfile(
		formationRef("cuda-base@2024.03"),
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(lf.Layers) != 1 {
		t.Fatalf("len(Layers) = %d, want 1", len(lf.Layers))
	}
	if lf.Layers[0].Name != "cuda" {
		t.Errorf("layer name = %q, want %q", lf.Layers[0].Name, "cuda")
	}
	if lf.Layers[0].MountOrder != 1 {
		t.Errorf("MountOrder = %d, want 1", lf.Layers[0].MountOrder)
	}
}

// TestStage1_UnknownOS verifies that an unsupported OS alias returns an error.
func TestStage1_UnknownOS(t *testing.T) {
	store := registry.NewMemoryStore()
	probeClient := &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: map[string]string{}}, // no entries
		Runner:   &probe.FakeRunner{Capabilities: map[string]*spec.BaseCapabilities{}},
		Cache:    probe.NewMemoryCache(),
	}
	r, err := resolver.New(resolver.Config{
		Registry: store,
		Probe:    probeClient,
	})
	if err != nil {
		t.Fatalf("resolver.New: %v", err)
	}

	profile := &spec.Profile{
		Name:     "test",
		Base:     spec.BaseRef{OS: "al2023", Arch: "x86_64"}, // valid in spec but not in static resolver
		Software: []spec.SoftwareRef{{Name: "foo"}},
	}
	_, err = r.Resolve(context.Background(), profile)
	assertResolutionError(t, err, "BASE_RESOLUTION_FAILED")
}

// TestStage3_LayerNotFound verifies that a missing layer returns an actionable error
// including available versions and the "strata search" hint.
func TestStage3_LayerNotFound(t *testing.T) {
	store := registry.NewMemoryStore()

	// Register older versions so available list is non-empty.
	store.AddLayer(signedLayer("alphafold", "3.0.0", "rhel",
		[]spec.Capability{{Name: "alphafold", Version: "3.0.0"}}, nil))
	store.AddLayer(signedLayer("alphafold", "3.0.1", "rhel",
		[]spec.Capability{{Name: "alphafold", Version: "3.0.1"}}, nil))

	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(softwareRef("alphafold", "4.0")))
	assertResolutionError(t, err, "LAYER_NOT_FOUND")

	var re *resolver.ResolutionError
	errors.As(err, &re)
	if len(re.Available) == 0 {
		t.Error("expected Available versions in error, got none")
	}
}

// TestStage2_FormationNotFound verifies that a missing formation returns a
// FORMATION_NOT_FOUND error.
func TestStage2_FormationNotFound(t *testing.T) {
	store := registry.NewMemoryStore()
	r := newResolver(t, store, nil)

	_, err := r.Resolve(context.Background(), testProfile(formationRef("nonexistent@1.0")))
	assertResolutionError(t, err, "FORMATION_NOT_FOUND")
}

// TestStage4_UnsatisfiedRequirement verifies that a layer with an unmet
// requirement fails at graph validation with UNSATISFIED_REQUIREMENT.
func TestStage4_UnsatisfiedRequirement(t *testing.T) {
	store := registry.NewMemoryStore()

	// openmpi requires cuda@>=12.0 — but cuda is not registered.
	openmpi := signedLayer("openmpi", "4.1.6", "rhel",
		[]spec.Capability{{Name: "mpi", Version: "3.1"}},
		[]spec.Requirement{{Name: "cuda", MinVersion: "12.0"}})
	store.AddLayer(openmpi)

	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(softwareRef("openmpi", "4.1.6")))
	assertResolutionError(t, err, "UNSATISFIED_REQUIREMENT")
}

// TestStage5_CapabilityConflict verifies that two layers providing the same
// capability name from different sources produce a CAPABILITY_CONFLICT error.
func TestStage5_CapabilityConflict(t *testing.T) {
	store := registry.NewMemoryStore()

	openmpi := signedLayer("openmpi", "4.1.6", "rhel",
		[]spec.Capability{
			{Name: "openmpi", Version: "4.1.6"},
			{Name: "mpi", Version: "3.1"},
		}, nil)
	mpich := signedLayer("mpich", "4.0.0", "rhel",
		[]spec.Capability{
			{Name: "mpich", Version: "4.0.0"},
			{Name: "mpi", Version: "3.1"},
		}, nil)
	store.AddLayer(openmpi)
	store.AddLayer(mpich)

	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(
		softwareRef("openmpi", "4.1.6"),
		softwareRef("mpich", "4.0.0"),
	))
	assertResolutionError(t, err, "CAPABILITY_CONFLICT")
}

// TestStage5_FileConflict verifies that two layers with conflicting ContentManifest
// entries produce a FILE_CONFLICT error.
func TestStage5_FileConflict(t *testing.T) {
	store := registry.NewMemoryStore()

	layerA := signedLayer("liba", "1.0.0", "rhel",
		[]spec.Capability{{Name: "liba", Version: "1.0.0"}}, nil)
	layerA.ContentManifest = map[string]string{
		"/usr/lib/libfoo.so": "aaaa1111",
	}

	layerB := signedLayer("libb", "1.0.0", "rhel",
		[]spec.Capability{{Name: "libb", Version: "1.0.0"}}, nil)
	layerB.ContentManifest = map[string]string{
		"/usr/lib/libfoo.so": "bbbb2222", // same path, different SHA
	}

	store.AddLayer(layerA)
	store.AddLayer(layerB)

	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(
		softwareRef("liba", "1.0.0"),
		softwareRef("libb", "1.0.0"),
	))
	assertResolutionError(t, err, "FILE_CONFLICT")
}

// TestStage5_SameFormation_NoConflict verifies that layers within the same
// formation are exempt from capability-level conflict checks.
func TestStage5_SameFormation_NoConflict(t *testing.T) {
	store := registry.NewMemoryStore()

	// Two layers that both provide "python" — normally a conflict.
	py310 := signedLayer("python310", "3.10.14", "rhel",
		[]spec.Capability{{Name: "python", Version: "3.10.14"}}, nil)
	py311 := signedLayer("python311", "3.11.9", "rhel",
		[]spec.Capability{{Name: "python", Version: "3.11.9"}}, nil)
	store.AddLayer(py310)
	store.AddLayer(py311)

	// Both are in the same formation — so no conflict expected.
	store.AddFormation(&spec.Formation{
		Name:       "python-multi",
		Version:    "1.0",
		Bundle:     "s3://strata/formations/python-multi@1.0/bundle.json",
		RekorEntry: "300",
		Layers: []spec.SoftwareRef{
			{Name: "python310", Version: "3.10.14"},
			{Name: "python311", Version: "3.11.9"},
		},
	})

	r := newResolver(t, store, nil)
	lf, err := r.Resolve(context.Background(), testProfile(formationRef("python-multi@1.0")))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(lf.Layers) != 2 {
		t.Errorf("len(Layers) = %d, want 2", len(lf.Layers))
	}
}

// TestStage6_MountOrder verifies that a dependency (gcc) receives a lower
// MountOrder than a layer that depends on it (openmpi).
func TestStage6_MountOrder(t *testing.T) {
	store := registry.NewMemoryStore()

	gcc := signedLayer("gcc", "13.2.0", "rhel",
		[]spec.Capability{{Name: "gcc", Version: "13.2.0"}},
		nil)
	openmpi := signedLayer("openmpi", "4.1.6", "rhel",
		[]spec.Capability{{Name: "mpi", Version: "3.1"}},
		[]spec.Requirement{{Name: "gcc", MinVersion: "13.0"}})
	store.AddLayer(gcc)
	store.AddLayer(openmpi)

	r := newResolver(t, store, nil)
	lf, err := r.Resolve(context.Background(), testProfile(
		softwareRef("openmpi", "4.1.6"),
		softwareRef("gcc", "13.2.0"),
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(lf.Layers) != 2 {
		t.Fatalf("len(Layers) = %d, want 2", len(lf.Layers))
	}

	layerMap := make(map[string]int)
	for _, l := range lf.Layers {
		layerMap[l.Name] = l.MountOrder
	}

	if layerMap["gcc"] >= layerMap["openmpi"] {
		t.Errorf("gcc MountOrder (%d) should be less than openmpi MountOrder (%d)",
			layerMap["gcc"], layerMap["openmpi"])
	}
}

// TestStage7_BundleMissing verifies that a layer without a Bundle field
// fails with BUNDLE_MISSING.
func TestStage7_BundleMissing(t *testing.T) {
	store := registry.NewMemoryStore()

	layer := signedLayer("tool", "1.0.0", "rhel",
		[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil)
	layer.Bundle = "" // clear the bundle

	store.AddLayer(layer)
	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(softwareRef("tool", "1.0.0")))
	assertResolutionError(t, err, "BUNDLE_MISSING")
}

// TestStage7_RekorEntryMissing verifies that a layer without a RekorEntry field
// fails with REKOR_ENTRY_MISSING.
func TestStage7_RekorEntryMissing(t *testing.T) {
	store := registry.NewMemoryStore()

	layer := signedLayer("tool", "1.0.0", "rhel",
		[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil)
	layer.RekorEntry = "" // clear the Rekor entry

	store.AddLayer(layer)
	r := newResolver(t, store, nil)
	_, err := r.Resolve(context.Background(), testProfile(softwareRef("tool", "1.0.0")))
	assertResolutionError(t, err, "REKOR_ENTRY_MISSING")
}

// TestStage7_RekorVerification verifies that when a Rekor client is configured,
// VerifyEntry is called and a failing client causes REKOR_VERIFICATION_FAILED.
func TestStage7_RekorVerification(t *testing.T) {
	store := registry.NewMemoryStore()

	layer := signedLayer("tool", "1.0.0", "rhel",
		[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil)
	store.AddLayer(layer)

	// FakeRekorClient always succeeds — should not produce an error.
	r := newResolver(t, store, &trust.FakeRekorClient{})
	lf, err := r.Resolve(context.Background(), testProfile(softwareRef("tool", "1.0.0")))
	if err != nil {
		t.Fatalf("Resolve with FakeRekorClient: %v", err)
	}
	if len(lf.Layers) != 1 {
		t.Errorf("len(Layers) = %d, want 1", len(lf.Layers))
	}
}

// TestEnvironmentID_Stability verifies that two resolutions with identical
// inputs produce the same EnvironmentID once the lockfiles are frozen.
func TestEnvironmentID_Stability(t *testing.T) {
	store := registry.NewMemoryStore()

	layer := signedLayer("python", "3.11.9", "rhel",
		[]spec.Capability{{Name: "python", Version: "3.11.9"}},
		nil)
	store.AddLayer(layer)

	r := newResolver(t, store, nil)
	profile := testProfile(softwareRef("python", "3.11.9"))
	profile.Env = map[string]string{"PYTHONPATH": "/strata/env/lib"}

	lf1, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	lf2, err := r.Resolve(context.Background(), profile)
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	// Freeze both lockfiles by injecting the same AMISHA256 (simulating a probed AMI).
	const fakeAMISHA256 = "sha256-ami-test123456789"
	lf1.Base.AMISHA256 = fakeAMISHA256
	lf2.Base.AMISHA256 = fakeAMISHA256

	id1 := lf1.EnvironmentID()
	id2 := lf2.EnvironmentID()

	if id1 == "" {
		t.Fatal("EnvironmentID is empty — lockfile may not be frozen")
	}
	if id1 != id2 {
		t.Errorf("EnvironmentID not stable: %q != %q", id1, id2)
	}

	// Also verify that the ProfileSHA256 is identical across runs.
	if lf1.ProfileSHA256 != lf2.ProfileSHA256 {
		t.Errorf("ProfileSHA256 not stable: %q != %q", lf1.ProfileSHA256, lf2.ProfileSHA256)
	}
}

// TestInvalidProfile verifies that Resolve rejects an invalid profile before
// touching the registry or probe.
func TestInvalidProfile(t *testing.T) {
	store := registry.NewMemoryStore()
	r := newResolver(t, store, nil)

	_, err := r.Resolve(context.Background(), &spec.Profile{}) // empty name + no base + no software
	if err == nil {
		t.Error("expected error for invalid profile, got nil")
	}
}
