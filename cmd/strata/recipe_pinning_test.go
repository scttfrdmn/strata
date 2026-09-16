// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/scttfrdmn/strata/internal/build"
)

// knownUnpinnedRecipes is the recorded #68 survey: the recipes whose build.sh
// fetches a source over the network without pinning it by digest
// (RecipeMeta.Sources). Measured 2026-09-16 — 32 of 33 recipes; the gap is
// catalog-wide, not two instances. Adoption (declaring Sources and having build.sh
// read the verified bytes from $STRATA_SOURCES) shrinks this list to empty, at
// which point the guard below becomes "every fetching recipe is pinned". It is a
// named-route list, not a silent gap: a new recipe that fetches without pinning
// fails the guard until it is either pinned or consciously added here.
var knownUnpinnedRecipes = []string{
	"application/julia/1.10.7",
	"application/julia/1.11.3",
	"application/miniforge/24.3.0",
	"application/quarto/1.4.555",
	"application/samtools/1.23",
	"core/cuda/11.8.0",
	"core/cuda/12.3.2",
	"core/cuda/12.6.0",
	"core/gcc/13.2.0",
	"core/gcc/14.2.0",
	"core/lmod/8.7.37",
	"core/nodejs/20.19.0",
	"core/nodejs/22.14.0",
	"core/rust/1.82.0",
	"library/fftw/3.3.10",
	"library/hdf5/1.12.3",
	"library/hdf5/1.14.4",
	"library/hwloc/2.11.2",
	"library/libfabric/1.22.0",
	"library/netcdf-c/4.9.2",
	"library/openblas/0.3.26",
	"library/openblas/0.3.28",
	"library/openmpi/5.0.10",
	"library/pmix/5.0.3",
	"library/ucx/1.17.0",
}

var recipeFetchRE = regexp.MustCompile(`(?m)\b(curl|wget)\b`)

// TestRecipeSourcePinning_Guard is the #68 regression tripwire: a recipe whose
// build.sh fetches from the network must either pin its sources (RecipeMeta.Sources,
// staged and verified by the build) or be on the known-unpinned list. A new
// unpinned recipe fails here (pin it), and pinning a listed recipe fails here too
// (remove it from the list) — so the survey cannot silently drift, in either
// direction.
func TestRecipeSourcePinning_Guard(t *testing.T) {
	const root = "recipes"

	known := make(map[string]bool, len(knownUnpinnedRecipes))
	for _, r := range knownUnpinnedRecipes {
		known[r] = true
	}

	got := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "build.sh" {
			return nil
		}
		dir := filepath.Dir(path)
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		sh, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !recipeFetchRE.Match(sh) {
			return nil // does not fetch — nothing to pin
		}
		recipe, parseErr := build.ParseRecipe(dir)
		if parseErr != nil {
			t.Errorf("recipe %q does not parse: %v", rel, parseErr)
			return nil
		}
		if len(recipe.Meta.Sources) == 0 {
			got[rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking recipes: %v", err)
	}

	// New regressions: fetches without pinning and not yet acknowledged.
	var regressions []string
	for r := range got {
		if !known[r] {
			regressions = append(regressions, r)
		}
	}
	sort.Strings(regressions)
	for _, r := range regressions {
		t.Errorf("recipe %q fetches a source without pinning it (RecipeMeta.Sources) — pin it so the layer is reproducible (#68), or add it to knownUnpinnedRecipes with a reason", r)
	}

	// Progress: listed as unpinned but no longer is — keep the survey honest.
	var pinned []string
	for r := range known {
		if !got[r] {
			pinned = append(pinned, r)
		}
	}
	sort.Strings(pinned)
	for _, r := range pinned {
		t.Errorf("recipe %q is on knownUnpinnedRecipes but now pins its sources (or no longer fetches) — remove it from the list", r)
	}
}
