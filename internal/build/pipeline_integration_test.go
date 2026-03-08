//go:build integration

package build

import (
	"context"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func TestRun_LocalBuild(t *testing.T) {
	// Requires mksquashfs + cosign on PATH.
	recipe := makeTestRecipe(t, "#!/bin/bash\ntouch \"$STRATA_PREFIX/hello.txt\"\n")
	reg := &mockPushRegistry{}
	executor := &LocalExecutor{Stdout: os.Stdout, Stderr: os.Stderr}
	signer := &trust.FakeSigner{}

	job := &Job{
		RecipeDir:   recipe.Dir,
		Base:        spec.BaseRef{OS: "al2023", Arch: "x86_64"},
		RegistryURL: "s3://test-bucket",
		DryRun:      false,
	}

	manifest, err := Run(context.Background(), job, recipe, reg, executor, signer)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if manifest.SHA256 == "" || manifest.SHA256 == "dry-run" {
		t.Errorf("expected real SHA256, got %q", manifest.SHA256)
	}
	if !reg.called {
		t.Error("expected PushLayer to be called")
	}
}
