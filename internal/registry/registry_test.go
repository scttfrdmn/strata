package registry_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func layer(name, version, arch, abi string) *spec.LayerManifest {
	return &spec.LayerManifest{
		ID:      name + "-" + version + "-" + abi + "-" + arch,
		Name:    name,
		Version: version,
		Arch:    arch,
		ABI:     abi,
		SHA256:  "sha256-" + version,
	}
}

func TestMemoryStoreResolveLayer(t *testing.T) {
	ctx := context.Background()
	s := registry.NewMemoryStore()

	s.AddLayer(layer("python", "3.11.9", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.8", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.12.0", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.9", "arm64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.9", "x86_64", "linux-gnu-2.35"))
	s.AddLayer(layer("cuda", "12.3.2", "x86_64", "linux-gnu-2.34"))

	tests := []struct {
		name, prefix, arch, family string
		wantVersion                string
		wantErr                    bool
	}{
		// No prefix → returns latest for arch/family.
		{"python", "", "x86_64", "linux-gnu-2.34", "3.12.0", false},
		// Prefix "3.11" → returns latest in that minor series.
		{"python", "3.11", "x86_64", "linux-gnu-2.34", "3.11.9", false},
		// Exact version prefix.
		{"python", "3.11.8", "x86_64", "linux-gnu-2.34", "3.11.8", false},
		// Different arch.
		{"python", "3.11", "arm64", "linux-gnu-2.34", "3.11.9", false},
		// Different family.
		{"python", "", "x86_64", "linux-gnu-2.35", "3.11.9", false},
		// cuda latest.
		{"cuda", "", "x86_64", "linux-gnu-2.34", "12.3.2", false},
		// Not found.
		{"alphafold", "", "x86_64", "linux-gnu-2.34", "", true},
		// Wrong arch.
		{"cuda", "", "arm64", "linux-gnu-2.34", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name+"@"+tt.prefix+"/"+tt.arch, func(t *testing.T) {
			m, err := s.ResolveLayer(ctx, tt.name, tt.prefix, tt.arch, tt.family)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveLayer(%q, %q, %q, %q) want error, got nil",
						tt.name, tt.prefix, tt.arch, tt.family)
				}
				if !registry.IsNotFound(err) {
					t.Errorf("error should be ErrNotFound, got %T: %v", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveLayer() unexpected error: %v", err)
			}
			if m.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", m.Version, tt.wantVersion)
			}
		})
	}
}

func TestMemoryStoreResolveFormation(t *testing.T) {
	ctx := context.Background()
	s := registry.NewMemoryStore()

	f := &spec.Formation{
		Name:    "cuda-python-ml",
		Version: "2024.03",
		Layers: []spec.SoftwareRef{
			{Name: "cuda", Version: "12.3"},
			{Name: "python", Version: "3.11"},
		},
	}
	s.AddFormation(f)

	got, err := s.ResolveFormation(ctx, "cuda-python-ml@2024.03", "x86_64")
	if err != nil {
		t.Fatalf("ResolveFormation() error: %v", err)
	}
	if got.Name != f.Name || got.Version != f.Version {
		t.Errorf("formation = %v, want %v", got, f)
	}

	_, err = s.ResolveFormation(ctx, "nonexistent@1.0", "x86_64")
	if err == nil || !registry.IsNotFound(err) {
		t.Error("ResolveFormation for missing formation should return ErrNotFound")
	}
}

func TestMemoryStoreBaseCapabilities(t *testing.T) {
	ctx := context.Background()
	s := registry.NewMemoryStore()

	caps := &spec.BaseCapabilities{
		AMIID: "ami-0abc123",
		OS:    "al2023",
		Arch:  "x86_64",
		ABI:   "linux-gnu-2.34",
	}

	// Not yet stored → ErrNotFound.
	_, err := s.GetBaseCapabilities(ctx, "ami-0abc123")
	if err == nil || !registry.IsNotFound(err) {
		t.Error("GetBaseCapabilities before store should return ErrNotFound")
	}

	if err := s.StoreBaseCapabilities(ctx, caps); err != nil {
		t.Fatalf("StoreBaseCapabilities() error: %v", err)
	}

	got, err := s.GetBaseCapabilities(ctx, "ami-0abc123")
	if err != nil {
		t.Fatalf("GetBaseCapabilities() after store error: %v", err)
	}
	if got.AMIID != caps.AMIID {
		t.Errorf("AMIID = %q, want %q", got.AMIID, caps.AMIID)
	}
}

func TestMemoryStoreListLayers(t *testing.T) {
	ctx := context.Background()
	s := registry.NewMemoryStore()

	s.AddLayer(layer("python", "3.12.0", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.9", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.8", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("cuda", "12.3.2", "x86_64", "linux-gnu-2.34"))
	s.AddLayer(layer("python", "3.11.9", "arm64", "linux-gnu-2.34"))

	// List all python x86_64 rhel → newest first.
	layers, err := s.ListLayers(ctx, "python", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("ListLayers() error: %v", err)
	}
	if len(layers) != 3 {
		t.Errorf("len(layers) = %d, want 3", len(layers))
	}
	if layers[0].Version != "3.12.0" {
		t.Errorf("first layer should be newest (3.12.0), got %q", layers[0].Version)
	}
	if layers[2].Version != "3.11.8" {
		t.Errorf("last layer should be oldest (3.11.8), got %q", layers[2].Version)
	}

	// List all layers (no filter).
	all, err := s.ListLayers(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListLayers() all error: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("len(all) = %d, want 5", len(all))
	}

	// List layers for a name with no match.
	empty, err := s.ListLayers(ctx, "alphafold", "", "")
	if err != nil {
		t.Fatalf("ListLayers() empty error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 layers for alphafold, got %d", len(empty))
	}
}

func TestIsNotFound(t *testing.T) {
	err := &registry.ErrNotFound{Kind: "layer", Key: "python@3.11"}
	if !registry.IsNotFound(err) {
		t.Error("ErrNotFound should be detected by IsNotFound")
	}
	if registry.IsNotFound(nil) {
		t.Error("IsNotFound(nil) should be false")
	}
}

func TestErrNotFoundMessage(t *testing.T) {
	err := &registry.ErrNotFound{Kind: "formation", Key: "r-research@2024.03"}
	want := "registry: formation \"r-research@2024.03\" not found"
	if err.Error() != want {
		t.Errorf("ErrNotFound.Error() = %q, want %q", err.Error(), want)
	}
}
