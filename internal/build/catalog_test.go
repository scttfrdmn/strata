package build

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRecipe creates a minimal recipe directory at dir/<name>/<version>/
// with build.sh and meta.yaml derived from the provided RecipeMeta.
func writeRecipe(t *testing.T, root, name, version string, meta string) string {
	t.Helper()
	dir := filepath.Join(root, name, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating recipe dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatalf("writing meta.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), []byte("#!/bin/bash\necho ok\n"), 0o755); err != nil {
		t.Fatalf("writing build.sh: %v", err)
	}
	return dir
}

func TestTopoSort_NoDependencies(t *testing.T) {
	deps := map[string][]string{
		"gcc":    {},
		"python": {},
		"cuda":   {},
	}
	stages, err := topoSort(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 1 {
		t.Fatalf("want 1 stage, got %d", len(stages))
	}
	if len(stages[0]) != 3 {
		t.Errorf("want 3 nodes in stage 0, got %d: %v", len(stages[0]), stages[0])
	}
}

func TestTopoSort_LinearChain(t *testing.T) {
	// gcc → openmpi → samtools (linear dependency chain)
	deps := map[string][]string{
		"gcc":      {},
		"openmpi":  {"gcc"},
		"samtools": {"openmpi"},
	}
	stages, err := topoSort(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 3 {
		t.Fatalf("want 3 stages, got %d: %v", len(stages), stages)
	}
	if len(stages[0]) != 1 || stages[0][0] != "gcc" {
		t.Errorf("stage 0: want [gcc], got %v", stages[0])
	}
	if len(stages[1]) != 1 || stages[1][0] != "openmpi" {
		t.Errorf("stage 1: want [openmpi], got %v", stages[1])
	}
	if len(stages[2]) != 1 || stages[2][0] != "samtools" {
		t.Errorf("stage 2: want [samtools], got %v", stages[2])
	}
}

func TestTopoSort_ParallelInStage(t *testing.T) {
	// gcc → {openmpi, python} — both can build in parallel after gcc
	deps := map[string][]string{
		"gcc":     {},
		"openmpi": {"gcc"},
		"python":  {"gcc"},
	}
	stages, err := topoSort(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("want 2 stages, got %d: %v", len(stages), stages)
	}
	if len(stages[0]) != 1 || stages[0][0] != "gcc" {
		t.Errorf("stage 0: want [gcc], got %v", stages[0])
	}
	if len(stages[1]) != 2 {
		t.Errorf("stage 1: want 2 nodes (openmpi, python), got %v", stages[1])
	}
}

func TestTopoSort_CycleDetected(t *testing.T) {
	deps := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	_, err := topoSort(deps)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestTopoSort_Empty(t *testing.T) {
	stages, err := topoSort(map[string][]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 0 {
		t.Errorf("want 0 stages, got %d", len(stages))
	}
}

func TestPlanCatalog_Empty(t *testing.T) {
	dir := t.TempDir()
	plan, err := PlanCatalog(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Stages) != 0 {
		t.Errorf("want 0 stages, got %d", len(plan.Stages))
	}
}

func TestPlanCatalog_TwoRecipes_NoDeps(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "gcc", "13.2.0", `
name: gcc
version: "13.2.0"
family: rhel
provides:
  - name: gcc
    version: "13.2.0"
`)
	writeRecipe(t, dir, "python", "3.11.9", `
name: python
version: "3.11.9"
family: rhel
provides:
  - name: python
    version: "3.11.9"
`)

	plan, err := PlanCatalog(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Recipes) != 2 {
		t.Fatalf("want 2 recipes, got %d", len(plan.Recipes))
	}
	if len(plan.Stages) != 1 {
		t.Fatalf("want 1 stage (no deps), got %d: %v", len(plan.Stages), plan.Stages)
	}
	if len(plan.Stages[0]) != 2 {
		t.Errorf("want 2 recipes in stage 0, got %v", plan.Stages[0])
	}
}

func TestPlanCatalog_WithDependency(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "gcc", "13.2.0", `
name: gcc
version: "13.2.0"
family: rhel
provides:
  - name: gcc
    version: "13.2.0"
  - name: gfortran
    version: "13.2.0"
`)
	writeRecipe(t, dir, "openmpi", "4.1.6", `
name: openmpi
version: "4.1.6"
family: rhel
provides:
  - name: openmpi
    version: "4.1.6"
build_requires:
  - name: gcc
    min_version: "13"
`)

	plan, err := PlanCatalog(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Stages) != 2 {
		t.Fatalf("want 2 stages, got %d: %v", len(plan.Stages), plan.Stages)
	}
	if plan.Stages[0][0] != "gcc" {
		t.Errorf("stage 0: want gcc, got %v", plan.Stages[0])
	}
	if plan.Stages[1][0] != "openmpi" {
		t.Errorf("stage 1: want openmpi, got %v", plan.Stages[1])
	}
}

func TestPlanCatalog_LatestVersionOnly(t *testing.T) {
	// Two versions of gcc; only the lexicographically latest (14.2.0) should be used.
	dir := t.TempDir()
	writeRecipe(t, dir, "gcc", "13.2.0", `
name: gcc
version: "13.2.0"
family: rhel
provides:
  - name: gcc
    version: "13.2.0"
`)
	writeRecipe(t, dir, "gcc", "14.2.0", `
name: gcc
version: "14.2.0"
family: rhel
provides:
  - name: gcc
    version: "14.2.0"
`)

	plan, err := PlanCatalog(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Recipes) != 1 {
		t.Fatalf("want 1 recipe (latest only), got %d", len(plan.Recipes))
	}
	r := plan.Recipes["gcc"]
	if r == nil {
		t.Fatal("gcc not in plan.Recipes")
	}
	if r.Meta.Version != "14.2.0" {
		t.Errorf("want version 14.2.0, got %s", r.Meta.Version)
	}
}
