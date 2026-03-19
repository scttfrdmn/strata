package strata_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	pkgstrata "github.com/scttfrdmn/strata/pkg/strata"
	"github.com/scttfrdmn/strata/spec"
)

func testProfile(arch string) *spec.Profile {
	return &spec.Profile{
		Name: "test",
		Base: spec.BaseRef{OS: "al2023", Arch: arch},
		Software: []spec.SoftwareRef{
			{Name: "gcc", Version: "13.2.0"},
		},
	}
}

func gccManifest(arch string) *spec.LayerManifest {
	return &spec.LayerManifest{
		Name:           "gcc",
		Version:        "13.2.0",
		Arch:           arch,
		ABI:            "linux-gnu-2.34",
		SHA256:         strings.Repeat("a", 64),
		Source:         "s3://strata-registry/layers/linux-gnu-2.34/" + arch + "/gcc/13.2.0/layer.sqfs",
		Bundle:         "s3://strata-registry/layers/linux-gnu-2.34/" + arch + "/gcc/13.2.0/bundle.json",
		RekorEntry:     "42",
		SignedBy:       "test@strata.dev",
		CosignVersion:  "v2.0.0",
		UserSelectable: true,
		InstallLayout:  "versioned",
		Provides: []spec.Capability{
			{Name: "gcc", Version: "13.2.0"},
			{Name: "c-compiler", Version: "13.2.0"},
		},
	}
}

func TestLockfileUserData_SmallLockfile(t *testing.T) {
	lf := &spec.LockFile{
		ProfileName: "test",
	}
	out, err := pkgstrata.LockfileUserData(lf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "profile: test") {
		t.Errorf("expected 'profile: test' in output, got: %q", out)
	}
}

func TestLockfileUserData_TooLarge(t *testing.T) {
	// Build a lockfile that exceeds 16 KB when YAML-encoded.
	lf := &spec.LockFile{
		ProfileName: strings.Repeat("x", 16*1024),
	}
	_, err := pkgstrata.LockfileUserData(lf)
	if err == nil {
		t.Fatal("expected error for oversized lockfile, got nil")
	}
	if !strings.Contains(err.Error(), "16 KB") {
		t.Errorf("expected '16 KB' in error message, got: %v", err)
	}
}

func TestResolve_Al2023(t *testing.T) {
	ctx := context.Background()

	store := registry.NewMemoryStore()
	store.AddLayer(gccManifest("x86_64"))

	c := pkgstrata.NewClientFromRegistry(store, "")

	profile := testProfile("x86_64")
	lf, err := c.Resolve(ctx, profile, pkgstrata.ResolveOptions{AMI: "ami-test"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if lf.ProfileName != "test" {
		t.Errorf("expected ProfileName 'test', got %q", lf.ProfileName)
	}
	if len(lf.Layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(lf.Layers))
	}
	if lf.Layers[0].Name != "gcc" {
		t.Errorf("expected gcc layer, got %q", lf.Layers[0].Name)
	}
}

func TestResolve_UnknownOS(t *testing.T) {
	ctx := context.Background()
	store := registry.NewMemoryStore()
	c := pkgstrata.NewClientFromRegistry(store, "")

	profile := &spec.Profile{
		Name:     "test",
		Base:     spec.BaseRef{OS: "unknownos", Arch: "x86_64"},
		Software: []spec.SoftwareRef{{Name: "gcc", Version: "13.2.0"}},
	}
	_, err := c.Resolve(ctx, profile, pkgstrata.ResolveOptions{})
	if err == nil {
		t.Fatal("expected error for unknown OS, got nil")
	}
}

func TestUploadLockfile_RequiresS3Client(t *testing.T) {
	ctx := context.Background()
	store := registry.NewMemoryStore()
	c := pkgstrata.NewClientFromRegistry(store, "")

	_, err := c.UploadLockfile(ctx, &spec.LockFile{ProfileName: "test"})
	if err == nil {
		t.Fatal("expected error when using non-S3 client, got nil")
	}
	if !strings.Contains(err.Error(), "S3-backed") {
		t.Errorf("expected 'S3-backed' in error, got: %v", err)
	}
}
