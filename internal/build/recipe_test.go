package build_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/spec"
)

// writeRecipe creates a valid recipe directory in dir and returns its path.
func writeRecipe(t *testing.T, dir string, meta string) string {
	t.Helper()
	recipeDir := filepath.Join(dir, "recipe")
	if err := os.MkdirAll(recipeDir, 0o750); err != nil {
		t.Fatalf("mkdir recipe: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "meta.yaml"), []byte(meta), 0o600); err != nil {
		t.Fatalf("write meta.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "build.sh"),
		[]byte("#!/bin/bash\nset -euo pipefail\necho 'build'\n"), 0o755); err != nil {
		t.Fatalf("write build.sh: %v", err)
	}
	return recipeDir
}

const validMeta = `
name: openmpi
version: 4.1.6
tier: library
description: Open MPI
provides:
  - name: openmpi
    version: 4.1.6
  - name: mpi
    version: "3.1"
build_requires:
  - name: gcc
    min_version: "13"
  - name: cuda
    min_version: "12.0"
runtime_requires:
  - name: glibc
    min_version: "2.34"
abi: linux-gnu-2.34
`

func TestParseRecipeValid(t *testing.T) {
	dir := t.TempDir()
	recipeDir := writeRecipe(t, dir, validMeta)

	r, err := build.ParseRecipe(recipeDir)
	if err != nil {
		t.Fatalf("ParseRecipe() error: %v", err)
	}
	if r.Meta.Name != "openmpi" {
		t.Errorf("Name = %q, want %q", r.Meta.Name, "openmpi")
	}
	if r.Meta.Version != "4.1.6" {
		t.Errorf("Version = %q, want %q", r.Meta.Version, "4.1.6")
	}
	if len(r.Meta.Provides) != 2 {
		t.Errorf("len(Provides) = %d, want 2", len(r.Meta.Provides))
	}
	if r.Meta.ABI != "linux-gnu-2.34" {
		t.Errorf("ABI = %q, want %q", r.Meta.ABI, "linux-gnu-2.34")
	}
	if r.BuildScriptPath == "" {
		t.Error("BuildScriptPath should not be empty")
	}
}

func TestParseRecipeMissingBuildSh(t *testing.T) {
	dir := t.TempDir()
	recipeDir := filepath.Join(dir, "recipe")
	if err := os.MkdirAll(recipeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "meta.yaml"), []byte(validMeta), 0o600); err != nil {
		t.Fatal(err)
	}
	// No build.sh
	if _, err := build.ParseRecipe(recipeDir); err == nil {
		t.Error("ParseRecipe without build.sh should return error")
	}
}

func TestParseRecipeMissingMeta(t *testing.T) {
	dir := t.TempDir()
	recipeDir := filepath.Join(dir, "recipe")
	if err := os.MkdirAll(recipeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := build.ParseRecipe(recipeDir); err == nil {
		t.Error("ParseRecipe without meta.yaml should return error")
	}
}

func TestRecipeMetaValidation(t *testing.T) {
	tests := []struct {
		name    string
		meta    build.RecipeMeta
		wantErr bool
	}{
		{
			name: "valid",
			meta: build.RecipeMeta{
				Name:    "python",
				Version: "3.11.9",
				Tier:    "core",
				Provides: []spec.Capability{
					{Name: "python", Version: "3.11.9"},
				},
				ABI: "linux-gnu-2.34",
			},
			wantErr: false,
		},
		{
			name:    "missing name",
			meta:    build.RecipeMeta{Version: "1.0", Tier: "core", Provides: []spec.Capability{{Name: "x"}}, ABI: "linux-gnu-2.34"},
			wantErr: true,
		},
		{
			name:    "missing version",
			meta:    build.RecipeMeta{Name: "x", Tier: "core", Provides: []spec.Capability{{Name: "x"}}, ABI: "linux-gnu-2.34"},
			wantErr: true,
		},
		{
			name:    "missing tier",
			meta:    build.RecipeMeta{Name: "x", Version: "1.0", Provides: []spec.Capability{{Name: "x"}}, ABI: "linux-gnu-2.34"},
			wantErr: true,
		},
		{
			name:    "invalid tier",
			meta:    build.RecipeMeta{Name: "x", Version: "1.0", Tier: "3", Provides: []spec.Capability{{Name: "x"}}, ABI: "linux-gnu-2.34"},
			wantErr: true,
		},
		{
			name: "core tier with build_requires",
			meta: build.RecipeMeta{
				Name: "x", Version: "1.0", Tier: "core", ABI: "linux-gnu-2.34",
				Provides:      []spec.Capability{{Name: "x"}},
				BuildRequires: []spec.Requirement{{Name: "gcc", MinVersion: "13"}},
			},
			wantErr: true,
		},
		{
			name:    "empty provides",
			meta:    build.RecipeMeta{Name: "x", Version: "1.0", Tier: "core", ABI: "linux-gnu-2.34"},
			wantErr: true,
		},
		{
			name:    "invalid family",
			meta:    build.RecipeMeta{Name: "x", Version: "1.0", Tier: "core", Provides: []spec.Capability{{Name: "x"}}, ABI: "windows"},
			wantErr: true,
		},
		{
			name: "provides entry with empty name",
			meta: build.RecipeMeta{
				Name:    "x",
				Version: "1.0",
				Tier:    "core",
				Provides: []spec.Capability{
					{Name: ""},
				},
				ABI: "linux-gnu-2.34",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestContentManifestConflicts(t *testing.T) {
	a := &build.ContentManifest{
		LayerID: "python",
		Files: map[string]string{
			"/usr/local/bin/python3": "sha-python",
			"/usr/local/lib/libz.so": "sha-libz",
		},
	}
	b := &build.ContentManifest{
		LayerID: "cuda",
		Files: map[string]string{
			"/usr/local/lib/libz.so": "sha-different-libz", // conflict
			"/usr/local/bin/nvcc":    "sha-nvcc",
		},
	}

	conflicts := a.ConflictsWith(b)
	if len(conflicts) != 1 {
		t.Errorf("ConflictsWith() = %v (len %d), want 1 conflict", conflicts, len(conflicts))
	}
	if len(conflicts) > 0 && conflicts[0] != "/usr/local/lib/libz.so" {
		t.Errorf("conflicting file = %q, want %q", conflicts[0], "/usr/local/lib/libz.so")
	}

	// Identical files (same path, same SHA256) are not conflicts.
	c := &build.ContentManifest{
		LayerID: "other",
		Files: map[string]string{
			"/usr/local/lib/libz.so": "sha-libz", // same SHA — no conflict
		},
	}
	if conflicts := a.ConflictsWith(c); len(conflicts) != 0 {
		t.Errorf("identical files should not conflict, got %v", conflicts)
	}

	// No overlap → no conflicts.
	d := &build.ContentManifest{
		LayerID: "unrelated",
		Files: map[string]string{
			"/opt/alphafold/run.py": "sha-af",
		},
	}
	if conflicts := a.ConflictsWith(d); len(conflicts) != 0 {
		t.Errorf("non-overlapping manifests should have no conflicts, got %v", conflicts)
	}
}

func TestJobValidation(t *testing.T) {
	tests := []struct {
		name    string
		job     build.Job
		wantErr bool
	}{
		{
			name: "valid",
			job: build.Job{
				RecipeDir:   "/some/path",
				Base:        spec.BaseRef{OS: "al2023"},
				RegistryURL: "s3://strata-layers",
			},
			wantErr: false,
		},
		{
			name:    "missing recipe dir",
			job:     build.Job{Base: spec.BaseRef{OS: "al2023"}, RegistryURL: "s3://bucket"},
			wantErr: true,
		},
		{
			name:    "invalid base OS",
			job:     build.Job{RecipeDir: "/path", Base: spec.BaseRef{OS: "windows"}, RegistryURL: "s3://bucket"},
			wantErr: true,
		},
		{
			name:    "non-S3 registry URL",
			job:     build.Job{RecipeDir: "/path", Base: spec.BaseRef{OS: "al2023"}, RegistryURL: "https://example.com"},
			wantErr: true,
		},
		{
			name:    "dry run skips registry URL check",
			job:     build.Job{RecipeDir: "/path", Base: spec.BaseRef{OS: "al2023"}, DryRun: true},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestSquashfsOptions(t *testing.T) {
	opts := build.SquashfsOptions()
	// Verify reproducible options are present.
	mustContain := []string{"-mkfs-time", "0", "-all-time", "0", "-noappend"}
	for _, want := range mustContain {
		found := false
		for _, opt := range opts {
			if opt == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SquashfsOptions() missing required flag %q", want)
		}
	}
}

func TestToLayerManifest(t *testing.T) {
	meta := &build.RecipeMeta{
		Name:    "openmpi",
		Version: "4.1.6",
		ABI:     "linux-gnu-2.34",
		Provides: []spec.Capability{
			{Name: "openmpi", Version: "4.1.6"},
			{Name: "mpi", Version: "3.1"},
		},
		RuntimeRequires: []spec.Requirement{
			{Name: "glibc", MinVersion: "2.34"},
		},
	}

	manifest := meta.ToLayerManifest("x86_64")
	if manifest.Name != "openmpi" {
		t.Errorf("Name = %q, want %q", manifest.Name, "openmpi")
	}
	if manifest.Version != "4.1.6" {
		t.Errorf("Version = %q, want %q", manifest.Version, "4.1.6")
	}
	if manifest.Arch != "x86_64" {
		t.Errorf("Arch = %q, want %q", manifest.Arch, "x86_64")
	}
	if manifest.ABI != "linux-gnu-2.34" {
		t.Errorf("ABI = %q, want %q", manifest.ABI, "linux-gnu-2.34")
	}
	if len(manifest.Provides) != 2 {
		t.Errorf("len(Provides) = %d, want 2", len(manifest.Provides))
	}
	if len(manifest.Requires) != 1 {
		t.Errorf("len(Requires) = %d, want 1", len(manifest.Requires))
	}
	// ID should be name-version-abi-arch.
	wantID := "openmpi-4.1.6-linux-gnu-2.34-x86_64"
	if manifest.ID != wantID {
		t.Errorf("ID = %q, want %q", manifest.ID, wantID)
	}
}

func TestRecipeMeta_UserSelectable_DefaultTrue(t *testing.T) {
	// When user_selectable is not set in YAML, ToLayerManifest should return UserSelectable=true.
	meta := &build.RecipeMeta{
		Name:    "python",
		Version: "3.11.9",
		ABI:     "linux-gnu-2.34",
		Provides: []spec.Capability{
			{Name: "python", Version: "3.11.9"},
		},
	}
	manifest := meta.ToLayerManifest("x86_64")
	if !manifest.UserSelectable {
		t.Error("UserSelectable should default to true when not explicitly set")
	}
}

func TestRecipeMeta_FlatLayout_Valid(t *testing.T) {
	meta := &build.RecipeMeta{
		Name:          "glibc",
		Version:       "2.34",
		Tier:          "core",
		ABI:           "linux-gnu-2.34",
		InstallLayout: "flat",
		Provides: []spec.Capability{
			{Name: "glibc", Version: "2.34"},
		},
	}
	if err := meta.Validate(); err != nil {
		t.Errorf("Validate() for flat layout: %v", err)
	}
	manifest := meta.ToLayerManifest("x86_64")
	if manifest.InstallLayout != "flat" {
		t.Errorf("InstallLayout = %q, want %q", manifest.InstallLayout, "flat")
	}
}
