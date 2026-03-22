package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func makeManifest(name, version string) *spec.LayerManifest {
	return &spec.LayerManifest{
		ID:      name + "-" + version + "-linux-gnu-2.34-x86_64",
		Name:    name,
		Version: version,
		ABI:     "linux-gnu-2.34",
		Arch:    "x86_64",
		SHA256:  "deadbeef",
		BuiltAt: time.Now().UTC(),
	}
}

func TestFederatedSearchOrder(t *testing.T) {
	// First store has the layer; second does not.
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	m := makeManifest("python", "3.12.13")
	first.AddLayer(m)

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	got, err := fed.ResolveLayer(ctx, "python", "3.12", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}
	if got.Version != "3.12.13" {
		t.Errorf("got version %q, want 3.12.13", got.Version)
	}
}

func TestFederatedFallthrough(t *testing.T) {
	// First store does not have the layer; second does.
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	m := makeManifest("python", "3.12.13")
	second.AddLayer(m)

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	got, err := fed.ResolveLayer(ctx, "python", "3.12", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("ResolveLayer fallthrough: %v", err)
	}
	if got.Version != "3.12.13" {
		t.Errorf("got version %q, want 3.12.13", got.Version)
	}
}

func TestFederatedAllNotFound(t *testing.T) {
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	_, err := fed.ResolveLayer(ctx, "python", "3.12", "x86_64", "linux-gnu-2.34")
	if !registry.IsNotFound(err) {
		t.Errorf("expected ErrNotFound when all registries miss, got %v", err)
	}
}

func TestFederatedListLayersMerged(t *testing.T) {
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	// first has python 3.12.13
	first.AddLayer(makeManifest("python", "3.12.13"))
	// second has python 3.12.13 (duplicate) and gcc 13.2.0
	second.AddLayer(makeManifest("python", "3.12.13"))
	second.AddLayer(makeManifest("gcc", "13.2.0"))

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	layers, err := fed.ListLayers(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListLayers: %v", err)
	}
	if len(layers) != 2 {
		t.Errorf("expected 2 unique layers (deduped), got %d", len(layers))
	}

	// Verify all IDs are unique.
	seen := make(map[string]int)
	for _, m := range layers {
		seen[m.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("layer ID %q appeared %d times (expected 1)", id, count)
		}
	}
}

func TestFederatedStoreCapabilitiesFirstOnly(t *testing.T) {
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	caps := &spec.BaseCapabilities{
		AMIID: "ami-test",
		OS:    "al2023",
		Arch:  "x86_64",
		ABI:   "linux-gnu-2.34",
	}

	if err := fed.StoreBaseCapabilities(ctx, caps); err != nil {
		t.Fatalf("StoreBaseCapabilities: %v", err)
	}

	// Should be in first registry.
	got, err := first.GetBaseCapabilities(ctx, "ami-test")
	if err != nil {
		t.Fatalf("first.GetBaseCapabilities: %v", err)
	}
	if got.AMIID != "ami-test" {
		t.Errorf("got AMIID %q", got.AMIID)
	}

	// Should NOT be in second registry.
	_, err = second.GetBaseCapabilities(ctx, "ami-test")
	if !registry.IsNotFound(err) {
		t.Errorf("expected second registry to not have capabilities, got err=%v", err)
	}
}

func TestFederatedFormationFallthrough(t *testing.T) {
	first := registry.NewMemoryStore()
	second := registry.NewMemoryStore()

	f := &spec.Formation{Name: "ml-stack", Version: "2024.03"}
	second.AddFormation(f)

	fed := registry.NewFederatedClient([]registry.Client{first, second})
	ctx := context.Background()

	got, err := fed.ResolveFormation(ctx, "ml-stack@2024.03", "x86_64")
	if err != nil {
		t.Fatalf("ResolveFormation fallthrough: %v", err)
	}
	if got.Name != "ml-stack" {
		t.Errorf("got name %q, want ml-stack", got.Name)
	}
}
