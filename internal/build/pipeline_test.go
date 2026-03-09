package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// mockPushRegistry records PushLayer calls but never actually uploads.
type mockPushRegistry struct {
	called   bool
	manifest *spec.LayerManifest
}

func (m *mockPushRegistry) PushLayer(_ context.Context, manifest *spec.LayerManifest, _ string, _ []byte) error {
	m.called = true
	m.manifest = manifest
	return nil
}

// makeTestRecipe creates a minimal recipe directory and returns a *Recipe.
func makeTestRecipe(t *testing.T, scriptContent string) *Recipe {
	t.Helper()
	dir := t.TempDir()
	meta := `name: test-layer
version: "1.0.0"
tier: "2"
family: rhel
provides:
  - name: test-layer
    version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatalf("writing meta.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("writing build.sh: %v", err)
	}
	recipe, err := ParseRecipe(dir)
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	return recipe
}

func TestRun_DryRun(t *testing.T) {
	recipe := makeTestRecipe(t, "#!/bin/bash\nexit 0\n")
	reg := &mockPushRegistry{}
	executor := &DryRunExecutor{Out: os.Stderr}
	signer := &trust.FakeSigner{}

	job := &Job{
		RecipeDir: recipe.Dir,
		Base:      spec.BaseRef{OS: "al2023", Arch: "x86_64"},
		DryRun:    true,
	}

	manifest, err := Run(context.Background(), job, recipe, reg, executor, signer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.SHA256 != "dry-run" {
		t.Errorf("expected SHA256 dry-run sentinel, got %q", manifest.SHA256)
	}
	if manifest.RekorEntry != "dry-run" {
		t.Errorf("expected RekorEntry dry-run sentinel, got %q", manifest.RekorEntry)
	}
	if reg.called {
		t.Error("PushLayer must not be called in dry-run mode")
	}
}

func TestRun_InvalidJob(t *testing.T) {
	recipe := makeTestRecipe(t, "#!/bin/bash\nexit 0\n")
	reg := &mockPushRegistry{}
	executor := &DryRunExecutor{}
	signer := &trust.FakeSigner{}

	// Empty RecipeDir is invalid.
	job := &Job{
		RecipeDir: "",
		Base:      spec.BaseRef{OS: "al2023"},
		DryRun:    false,
	}

	_, err := Run(context.Background(), job, recipe, reg, executor, signer)
	if err == nil {
		t.Error("expected error for invalid job")
	}
}
