package build

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Plan is a topologically ordered build plan for a catalog of recipes.
// Recipes in the same Stage can build in parallel on independent EC2 instances.
// Each stage must complete before the next starts, because later stages may
// depend on layers produced by earlier stages.
type Plan struct {
	// Stages is the ordered list of build stages. Each stage is a set of
	// recipe names that can build concurrently.
	Stages [][]string

	// Recipes maps recipe name to its parsed recipe.
	Recipes map[string]*Recipe
}

// PlanCatalog parses all recipes in recipesDir and returns a BuildPlan with
// a topologically sorted build order. Recipes that share no dependency
// relationship appear in the same stage and can build in parallel.
//
// recipesDir is expected to contain subdirectories named after each recipe,
// each containing version subdirectories (e.g. gcc/13.2.0/). The latest
// version of each recipe is used.
func PlanCatalog(recipesDir string) (*Plan, error) {
	recipes, err := discoverRecipes(recipesDir)
	if err != nil {
		return nil, fmt.Errorf("catalog: discovering recipes in %q: %w", recipesDir, err)
	}
	if len(recipes) == 0 {
		return &Plan{Recipes: map[string]*Recipe{}}, nil
	}

	// Build a provides index: capability name → recipe name.
	// Used to resolve build_requires to recipe dependencies.
	provides := make(map[string]string) // capability -> recipe name
	for name, r := range recipes {
		for _, cap := range r.Meta.Provides {
			provides[cap.Name] = name
		}
	}

	// Build a dependency graph: recipe name → set of recipe names it depends on.
	deps := make(map[string][]string)
	for name, r := range recipes {
		var d []string
		for _, req := range r.Meta.BuildRequires {
			if dep, ok := provides[req.Name]; ok && dep != name {
				d = append(d, dep)
			}
		}
		sort.Strings(d) // deterministic order
		deps[name] = d
	}

	// Topological sort (Kahn's algorithm) producing stages.
	stages, err := topoSort(deps)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}

	return &Plan{
		Stages:  stages,
		Recipes: recipes,
	}, nil
}

// discoverRecipes finds all recipes in recipesDir. Each recipe is expected
// at recipesDir/<name>/<version>/ containing build.sh and meta.yaml.
// For each name, only the lexicographically latest version directory is used.
func discoverRecipes(dir string) (map[string]*Recipe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	recipes := make(map[string]*Recipe)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		nameDir := filepath.Join(dir, name)

		// Find the latest version subdirectory.
		versions, err := os.ReadDir(nameDir)
		if err != nil {
			continue
		}
		var latestVersion string
		for _, v := range versions {
			if v.IsDir() && v.Name() > latestVersion {
				latestVersion = v.Name()
			}
		}
		if latestVersion == "" {
			continue
		}

		recipeDir := filepath.Join(nameDir, latestVersion)
		r, err := ParseRecipe(recipeDir)
		if err != nil {
			return nil, fmt.Errorf("parsing recipe at %q: %w", recipeDir, err)
		}
		recipes[name] = r
	}
	return recipes, nil
}

// topoSort performs Kahn's topological sort on the dependency graph, grouping
// nodes with the same depth into stages. Returns an error if cycles exist.
// inDegree[n] = number of unsatisfied dependencies for n; nodes with
// inDegree=0 are ready to build.
func topoSort(deps map[string][]string) ([][]string, error) {
	// Build inDegree map: for each node, count how many of its dependencies
	// have not yet been built. Nodes that are depended upon but not keys in
	// deps (i.e. external layers) are registered with inDegree=0.
	inDegree := make(map[string]int)
	for name, depList := range deps {
		inDegree[name] = len(depList)
		for _, dep := range depList {
			if _, exists := inDegree[dep]; !exists {
				inDegree[dep] = 0
			}
		}
	}

	var stages [][]string
	remaining := len(inDegree)

	for remaining > 0 {
		// Collect all nodes with in-degree 0.
		var stage []string
		for name, deg := range inDegree {
			if deg == 0 {
				stage = append(stage, name)
			}
		}
		if len(stage) == 0 {
			return nil, fmt.Errorf("cycle detected in build dependency graph")
		}
		sort.Strings(stage) // deterministic ordering within stage
		stages = append(stages, stage)

		// Remove processed nodes and update in-degrees.
		staged := make(map[string]bool)
		for _, n := range stage {
			staged[n] = true
			delete(inDegree, n)
			remaining--
		}

		// Recompute in-degree for each remaining node as the number of its
		// dependencies that have not yet been processed (still in inDegree).
		// Staged nodes were deleted from inDegree above, so checking existence
		// in inDegree correctly filters both current and prior-stage completions.
		for name := range inDegree {
			count := 0
			for _, dep := range deps[name] {
				if _, pending := inDegree[dep]; pending {
					count++
				}
			}
			inDegree[name] = count
		}
	}

	return stages, nil
}
