package examples_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/spec"
)

// TestAllRecipesParse validates that every recipe directory under
// cmd/strata/recipes/ has a well-formed meta.yaml and a build.sh that exists.
// This acts as a catalog correctness gate: CI fails if any recipe is broken.
func TestAllRecipesParse(t *testing.T) {
	recipesRoot := filepath.Join("..", "cmd", "strata", "recipes")

	// Walk two levels: <name>/<version>/
	nameDirs, err := os.ReadDir(recipesRoot)
	if err != nil {
		t.Fatalf("reading recipes dir %q: %v", recipesRoot, err)
	}
	if len(nameDirs) == 0 {
		t.Fatalf("no recipe name directories found under %q", recipesRoot)
	}

	total := 0
	for _, nameEntry := range nameDirs {
		if !nameEntry.IsDir() {
			continue
		}
		nameDir := filepath.Join(recipesRoot, nameEntry.Name())

		versionDirs, err := os.ReadDir(nameDir)
		if err != nil {
			t.Errorf("reading name dir %q: %v", nameDir, err)
			continue
		}

		for _, verEntry := range versionDirs {
			if !verEntry.IsDir() {
				continue
			}
			recipeDir := filepath.Join(nameDir, verEntry.Name())
			total++

			t.Run(nameEntry.Name()+"/"+verEntry.Name(), func(t *testing.T) {
				recipe, err := build.ParseRecipe(recipeDir)
				if err != nil {
					t.Fatalf("ParseRecipe(%q): %v", recipeDir, err)
				}
				if recipe.Meta.Name == "" {
					t.Error("Name is empty")
				}
				if recipe.Meta.Version == "" {
					t.Error("Version is empty")
				}
				if len(recipe.Meta.Provides) == 0 {
					t.Error("Provides is empty")
				}
				if recipe.Meta.Family == "" {
					t.Error("Family is empty")
				}
			})
		}
	}

	if total == 0 {
		t.Fatal("no recipe version directories found — check recipes directory structure")
	}
}

// TestAllFormationsParse validates that every formation YAML file under
// cmd/strata/formations/ parses as a well-formed spec.Formation with
// non-empty Name, Version, and at least one Layer.
func TestAllFormationsParse(t *testing.T) {
	formationsRoot := filepath.Join("..", "cmd", "strata", "formations")

	entries, err := os.ReadDir(formationsRoot)
	if err != nil {
		t.Fatalf("reading formations dir %q: %v", formationsRoot, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no formation files found under %q", formationsRoot)
	}

	total := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		total++

		t.Run(entry.Name(), func(t *testing.T) {
			path := filepath.Join(formationsRoot, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %q: %v", path, err)
			}

			var f spec.Formation
			if err := yaml.Unmarshal(data, &f); err != nil {
				t.Fatalf("parsing %q: %v", path, err)
			}
			if f.Name == "" {
				t.Error("Name is empty")
			}
			if f.Version == "" {
				t.Error("Version is empty")
			}
			if len(f.Layers) == 0 {
				t.Error("Layers is empty")
			}
			for i, layer := range f.Layers {
				if layer.Name == "" && layer.Formation == "" {
					t.Errorf("Layers[%d]: both Name and Formation are empty", i)
				}
			}
		})
	}

	if total == 0 {
		t.Fatal("no .yaml formation files found — check formations directory structure")
	}
}
