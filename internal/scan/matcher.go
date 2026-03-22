package scan

import (
	"context"
	"strings"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// MatchAll matches detected packages against a registry.
// Name comparison: case-insensitive, hyphen/underscore equivalent (pip normalization).
// Version comparison: exact string match.
// Results are in the same order as pkgs.
func MatchAll(ctx context.Context, reg registry.Client, pkgs []DetectedPackage, arch, abi string) ([]MatchResult, error) {
	// Group by unique normalized name to minimize ListLayers calls.
	type cached struct {
		manifests []*spec.LayerManifest
		err       error
	}

	nameSet := make(map[string]bool)
	for _, p := range pkgs {
		nameSet[normalizeName(p.Name)] = true
	}

	cache := make(map[string]cached, len(nameSet))
	for normName := range nameSet {
		mfs, err := reg.ListLayers(ctx, normName, arch, abi)
		cache[normName] = cached{manifests: mfs, err: err}
	}

	results := make([]MatchResult, 0, len(pkgs))
	for _, p := range pkgs {
		norm := normalizeName(p.Name)
		cr := cache[norm]
		mr := MatchResult{Package: p}

		if cr.err != nil || len(cr.manifests) == 0 {
			mr.Status = StatusUnmatched
		} else {
			var exactMatch *spec.LayerManifest
			for _, m := range cr.manifests {
				if m.Version == p.Version {
					exactMatch = m
					break
				}
			}
			if exactMatch != nil {
				mr.Status = StatusMatched
				mr.Manifest = exactMatch
			} else {
				mr.Status = StatusNearMatch
				mr.Manifest = cr.manifests[0]
				mr.NearVersion = cr.manifests[0].Version
			}
		}
		results = append(results, mr)
	}

	return results, nil
}

// normalizeName applies pip-style normalization: lowercase, hyphens and underscores equivalent.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	return name
}
