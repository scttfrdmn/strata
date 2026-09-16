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
var knownUnpinnedRecipes = []string{}

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
