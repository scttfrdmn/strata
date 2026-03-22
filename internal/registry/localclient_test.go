package registry_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func makeTestManifest(name, version, abi, arch string) *spec.LayerManifest {
	return &spec.LayerManifest{
		ID:      name + "-" + version + "-" + abi + "-" + arch,
		Name:    name,
		Version: version,
		ABI:     abi,
		Arch:    arch,
		SHA256:  "aabbccdd",
		Size:    1024,
		BuiltAt: time.Now().UTC(),
		Provides: []spec.Capability{
			{Name: name, Version: version},
		},
		UserSelectable: true,
	}
}

func writeYAMLFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile %q: %v", path, err)
	}
}

func TestNewLocalClientFromFileURL(t *testing.T) {
	dir := t.TempDir()
	url := "file://" + dir
	c, err := registry.NewLocalClient(url)
	if err != nil {
		t.Fatalf("NewLocalClient(%q) error: %v", url, err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	for _, sub := range []string{"layers", "formations", "probes", "index", "locks"} {
		info, statErr := os.Stat(filepath.Join(dir, sub))
		if statErr != nil || !info.IsDir() {
			t.Errorf("expected subdirectory %q to exist", sub)
		}
	}
}

func TestLocalClientNotFound(t *testing.T) {
	c, err := registry.NewLocalClient("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	_, err = c.ResolveLayer(ctx, "python", "3.12", "x86_64", "linux-gnu-2.34")
	if !registry.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	_, err = c.ResolveFormation(ctx, "ml-stack@2024.03", "x86_64")
	if !registry.IsNotFound(err) {
		t.Errorf("expected ErrNotFound for formation, got %v", err)
	}

	_, err = c.GetBaseCapabilities(ctx, "ami-unknown")
	if !registry.IsNotFound(err) {
		t.Errorf("expected ErrNotFound for capabilities, got %v", err)
	}
}

func TestLocalClientPushAndResolve(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sqfsPath := filepath.Join(dir, "test.sqfs")
	if err := os.WriteFile(sqfsPath, []byte("fake-squashfs-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := makeTestManifest("python", "3.12.13", "linux-gnu-2.34", "x86_64")

	if err := c.PushLayer(ctx, m, sqfsPath, []byte(`{"bundle":"test"}`)); err != nil {
		t.Fatalf("PushLayer: %v", err)
	}

	got, err := c.ResolveLayer(ctx, "python", "3.12", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}
	if got.Version != "3.12.13" {
		t.Errorf("got version %q, want 3.12.13", got.Version)
	}
	if got.Source == "" {
		t.Error("Source should be set after push")
	}
}

func TestLocalClientRebuildIndex(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sqfsPath := filepath.Join(dir, "test.sqfs")
	if err := os.WriteFile(sqfsPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	m1 := makeTestManifest("gcc", "13.2.0", "linux-gnu-2.34", "x86_64")
	m2 := makeTestManifest("python", "3.12.13", "linux-gnu-2.34", "x86_64")

	if err := c.PushLayer(ctx, m1, sqfsPath, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := c.PushLayer(ctx, m2, sqfsPath, []byte("{}")); err != nil {
		t.Fatal(err)
	}

	// Delete the index and rebuild from scratch.
	os.Remove(filepath.Join(dir, "index", "layers.yaml")) //nolint:errcheck
	if err := c.RebuildIndex(ctx); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	layers, err := c.ListLayers(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Errorf("expected 2 layers after rebuild, got %d", len(layers))
	}
}

func TestLocalClientFetchLayerSqfs(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sqfsData := []byte("squashfs-content-for-fetch-test")
	sqfsPath := filepath.Join(dir, "test.sqfs")
	if err := os.WriteFile(sqfsPath, sqfsData, 0o644); err != nil {
		t.Fatal(err)
	}
	sha256hex, err := registry.SHA256HexFile(sqfsPath)
	if err != nil {
		t.Fatal(err)
	}

	m := makeTestManifest("gcc", "13.2.0", "linux-gnu-2.34", "x86_64")
	m.SHA256 = sha256hex

	if err := c.PushLayer(ctx, m, sqfsPath, []byte("{}")); err != nil {
		t.Fatalf("PushLayer: %v", err)
	}

	cacheDir := t.TempDir()
	gotPath, err := c.FetchLayerSqfs(ctx, m, cacheDir)
	if err != nil {
		t.Fatalf("FetchLayerSqfs: %v", err)
	}

	actual, err := registry.SHA256HexFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if actual != sha256hex {
		t.Errorf("fetched file SHA256 %q != expected %q", actual, sha256hex)
	}

	// Second fetch should hit the cache.
	gotPath2, err := c.FetchLayerSqfs(ctx, m, cacheDir)
	if err != nil {
		t.Fatalf("second FetchLayerSqfs: %v", err)
	}
	if gotPath2 != gotPath {
		t.Errorf("cached fetch returned different path: %q vs %q", gotPath2, gotPath)
	}
}

func TestLocalClientFormations(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	_, err = c.ResolveFormation(ctx, "ml-stack@2024.03", "x86_64")
	if !registry.IsNotFound(err) {
		t.Errorf("expected not found before write, got %v", err)
	}

	// Write a formation manually to verify the read path.
	formationDir := filepath.Join(dir, "formations", "ml-stack", "2024.03")
	if err := os.MkdirAll(formationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := spec.Formation{Name: "ml-stack", Version: "2024.03"}
	writeYAMLFile(t, filepath.Join(formationDir, "manifest.yaml"), f)

	got, err := c.ResolveFormation(ctx, "ml-stack@2024.03", "x86_64")
	if err != nil {
		t.Fatalf("ResolveFormation: %v", err)
	}
	if got.Name != "ml-stack" {
		t.Errorf("got formation name %q, want ml-stack", got.Name)
	}
}

func TestLocalClientBaseCapabilities(t *testing.T) {
	c, err := registry.NewLocalClient("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	caps := &spec.BaseCapabilities{
		AMIID:    "ami-test123",
		OS:       "al2023",
		Arch:     "x86_64",
		ABI:      "linux-gnu-2.34",
		ProbedAt: time.Now().UTC(),
	}

	if err := c.StoreBaseCapabilities(ctx, caps); err != nil {
		t.Fatalf("StoreBaseCapabilities: %v", err)
	}

	got, err := c.GetBaseCapabilities(ctx, "ami-test123")
	if err != nil {
		t.Fatalf("GetBaseCapabilities: %v", err)
	}
	if got.AMIID != "ami-test123" {
		t.Errorf("got AMIID %q, want ami-test123", got.AMIID)
	}
}

func TestLocalClientLockfileRoundTrip(t *testing.T) {
	c, err := registry.NewLocalClient("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	lf := &spec.LockFile{
		ProfileName: "test",
		Base: spec.ResolvedBase{
			AMISHA256: "deadbeef",
			AMIID:     "ami-test",
		},
	}

	uri, err := c.PutLockfile(ctx, lf)
	if err != nil {
		t.Fatalf("PutLockfile: %v", err)
	}
	if uri == "" {
		t.Error("expected non-empty URI")
	}

	records, err := c.ListLockfiles(ctx)
	if err != nil {
		t.Fatalf("ListLockfiles: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 lockfile, got %d", len(records))
	}
	if records[0].LockFile.ProfileName != "test" {
		t.Errorf("got profile name %q, want test", records[0].LockFile.ProfileName)
	}
}

func TestLocalClientConcurrentPush(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sqfsPath := filepath.Join(dir, "test.sqfs")
	if err := os.WriteFile(sqfsPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	versions := []string{"1.0.0", "1.0.1", "1.0.2", "1.0.3"}
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m := makeTestManifest("pkg", versions[idx], "linux-gnu-2.34", "x86_64")
			errs[idx] = c.PushLayer(ctx, m, sqfsPath, []byte("{}"))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: PushLayer error: %v", i, err)
		}
	}

	layers, err := c.ListLayers(ctx, "pkg", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != n {
		t.Errorf("expected %d layers after concurrent push, got %d", n, len(layers))
	}
}
