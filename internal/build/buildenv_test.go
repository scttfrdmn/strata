package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// fakeRegistryClient implements BuildRegistryClient for testing.
type fakeRegistryClient struct {
	manifests map[string]*spec.LayerManifest // "name@version" -> manifest
	sqfsPaths map[string]string              // manifest.ID -> sqfs path
	fetchErr  error
}

func (f *fakeRegistryClient) ResolveLayer(_ context.Context, name, versionPrefix, _, _ string) (*spec.LayerManifest, error) {
	key := name
	if versionPrefix != "" {
		key += "@" + versionPrefix
	}
	m, ok := f.manifests[key]
	if !ok {
		m, ok = f.manifests[name]
	}
	if !ok {
		return nil, errors.New("fakeRegistryClient: layer not found: " + key)
	}
	return m, nil
}

func (f *fakeRegistryClient) FetchLayerSqfs(_ context.Context, manifest *spec.LayerManifest, cacheDir string) (string, error) {
	if f.fetchErr != nil {
		return "", f.fetchErr
	}
	path, ok := f.sqfsPaths[manifest.ID]
	if !ok {
		return "", errors.New("fakeRegistryClient: no sqfs for " + manifest.ID)
	}
	// Copy the file to cacheDir so callers see a real file.
	dst := filepath.Join(cacheDir, manifest.SHA256+".sqfs")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func TestRegistryBuildEnvResolver_Resolve(t *testing.T) {
	dir := t.TempDir()

	// Create a fake .sqfs file.
	sqfsPath := filepath.Join(dir, "gcc.sqfs")
	if err := os.WriteFile(sqfsPath, []byte("fake-sqfs"), 0o644); err != nil {
		t.Fatal(err)
	}

	gccManifest := &spec.LayerManifest{
		ID:      "gcc-13.2.0-rhel-x86_64",
		Name:    "gcc",
		Version: "13.2.0",
		Arch:    "x86_64",
		Family:  "rhel",
		SHA256:  "abc123",
		Source:  "s3://strata-registry/layers/rhel/x86_64/gcc/13.2.0/layer.sqfs",
	}

	client := &fakeRegistryClient{
		manifests: map[string]*spec.LayerManifest{
			"gcc": gccManifest,
		},
		sqfsPaths: map[string]string{
			gccManifest.ID: sqfsPath,
		},
	}

	resolver := &RegistryBuildEnvResolver{Registry: client}
	cacheDir := filepath.Join(dir, "cache")

	layers, err := resolver.Resolve(context.Background(),
		[]spec.Requirement{{Name: "gcc", MinVersion: "13.0"}},
		"x86_64", "rhel", cacheDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}
	if layers[0].Manifest.ID != gccManifest.ID {
		t.Errorf("manifest ID = %q, want %q", layers[0].Manifest.ID, gccManifest.ID)
	}
	if layers[0].MountOrder != 1 {
		t.Errorf("MountOrder = %d, want 1", layers[0].MountOrder)
	}
	if _, err := os.Stat(layers[0].SqfsPath); err != nil {
		t.Errorf("sqfs file missing: %v", err)
	}
}

func TestRegistryBuildEnvResolver_MissingLayer(t *testing.T) {
	client := &fakeRegistryClient{manifests: map[string]*spec.LayerManifest{}}
	resolver := &RegistryBuildEnvResolver{Registry: client}
	_, err := resolver.Resolve(context.Background(),
		[]spec.Requirement{{Name: "gcc"}},
		"x86_64", "rhel", t.TempDir())
	if err == nil {
		t.Error("expected error for missing layer, got nil")
	}
}

func TestRegistryBuildEnvResolver_MultipleRequires(t *testing.T) {
	dir := t.TempDir()

	makeManifest := func(name, ver string) *spec.LayerManifest {
		return &spec.LayerManifest{
			ID:      name + "-" + ver + "-rhel-x86_64",
			Name:    name,
			Version: ver,
			SHA256:  name + "-hash",
		}
	}
	makeFile := func(name string) string {
		p := filepath.Join(dir, name+".sqfs")
		_ = os.WriteFile(p, []byte("x"), 0o644)
		return p
	}

	gccM := makeManifest("gcc", "13.2.0")
	openmpiM := makeManifest("openmpi", "5.0.6")

	client := &fakeRegistryClient{
		manifests: map[string]*spec.LayerManifest{
			"gcc":     gccM,
			"openmpi": openmpiM,
		},
		sqfsPaths: map[string]string{
			gccM.ID:     makeFile("gcc"),
			openmpiM.ID: makeFile("openmpi"),
		},
	}

	resolver := &RegistryBuildEnvResolver{Registry: client}
	layers, err := resolver.Resolve(context.Background(),
		[]spec.Requirement{{Name: "gcc"}, {Name: "openmpi"}},
		"x86_64", "rhel", filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	if layers[0].MountOrder != 1 || layers[1].MountOrder != 2 {
		t.Errorf("MountOrder = [%d, %d], want [1, 2]", layers[0].MountOrder, layers[1].MountOrder)
	}
}

func TestFakeBuildEnvResolver_Resolve(t *testing.T) {
	gccLayer := EnvLayer{
		Manifest: &spec.LayerManifest{ID: "gcc-13.2.0-rhel-x86_64"},
		SqfsPath: "/tmp/gcc.sqfs",
	}
	resolver := &FakeBuildEnvResolver{Layers: map[string]EnvLayer{"gcc": gccLayer}}

	layers, err := resolver.Resolve(context.Background(),
		[]spec.Requirement{{Name: "gcc", MinVersion: "13.0"}},
		"x86_64", "rhel", t.TempDir())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(layers) != 1 || layers[0].Manifest.ID != gccLayer.Manifest.ID {
		t.Errorf("unexpected layer: %v", layers)
	}
	if layers[0].MountOrder != 1 {
		t.Errorf("MountOrder = %d, want 1", layers[0].MountOrder)
	}
}

func TestFakeBuildEnvResolver_MissingLayer(t *testing.T) {
	resolver := &FakeBuildEnvResolver{Layers: map[string]EnvLayer{}}
	_, err := resolver.Resolve(context.Background(),
		[]spec.Requirement{{Name: "gcc"}},
		"x86_64", "rhel", t.TempDir())
	if err == nil {
		t.Error("expected error for missing layer")
	}
}
