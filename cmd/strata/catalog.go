package main

import (
	"embed"
	"io/fs"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

//go:embed recipes formations
var catalogFS embed.FS

// buildCatalog walks the embedded recipe and formation directories,
// parses every meta.yaml and formation YAML, and returns a MemoryStore
// seeded with all known layers and formations.
//
// Parse errors are silently skipped so CLI startup never panics on a
// malformed catalog file. The examples/catalog_test.go validates all
// files parse cleanly at test time.
func buildCatalog() *registry.MemoryStore {
	store := registry.NewMemoryStore()

	// Walk recipes/<tier>/<name>/<version>/meta.yaml → two LayerManifests (x86_64 + arm64).
	entries, _ := fs.Glob(catalogFS, "recipes/*/*/*/meta.yaml")
	for _, path := range entries {
		data, err := catalogFS.ReadFile(path)
		if err != nil {
			continue
		}
		var meta build.RecipeMeta
		if yaml.Unmarshal(data, &meta) != nil {
			continue
		}
		if meta.Validate() != nil {
			continue
		}
		for _, arch := range []string{"x86_64", "arm64"} {
			store.AddLayer(meta.ToLayerManifest(arch))
		}
	}

	// Walk formations/*.yaml → spec.Formation entries.
	entries, _ = fs.Glob(catalogFS, "formations/*.yaml")
	for _, path := range entries {
		data, err := catalogFS.ReadFile(path)
		if err != nil {
			continue
		}
		var f spec.Formation
		if yaml.Unmarshal(data, &f) != nil {
			continue
		}
		if f.Name == "" || f.Version == "" {
			continue
		}
		store.AddFormation(&f)
	}

	return store
}
