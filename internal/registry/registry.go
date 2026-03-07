// Package registry implements the Strata layer catalog client.
//
// The registry is the S3-backed catalog of signed layer manifests, formation
// manifests, and base capability probes. It is the translation layer between
// user intent (software refs in a profile) and system artifacts (layer
// manifests in a lockfile).
//
// The resolver queries the registry during resolution; it never directly
// accesses S3 URLs. The registry enforces that all returned manifests are
// structurally valid — it does not verify Sigstore signatures (that is the
// trust package's job, called by the resolver after manifest fetch).
//
// # Catalog layout (S3)
//
//	s3://<bucket>/
//	  layers/
//	    <family>/<arch>/<name>/<version>/
//	      manifest.yaml     — LayerManifest
//	      layer.sqfs        — squashfs image
//	      bundle.json       — cosign bundle
//	  formations/
//	    <name>/<version>/
//	      manifest.yaml     — Formation
//	      bundle.json       — cosign bundle
//	  probes/
//	    <ami-id>/
//	      capabilities.yaml — BaseCapabilities
//	  index/
//	    layers.yaml         — flat catalog index for fast prefix matching
package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/scttfrdmn/strata/spec"
)

// Client is the interface the resolver uses to query the layer catalog.
// All methods operate on name/version/arch/family coordinates, not raw
// S3 URLs — those are the registry's internal concern.
type Client interface {
	// ResolveLayer returns the best-matching LayerManifest for the given
	// coordinates. versionPrefix is a semver prefix (e.g. "12.3" matches
	// "12.3.0", "12.3.1", …). An empty versionPrefix returns the latest
	// stable version. Returns ErrNotFound if no match exists.
	ResolveLayer(ctx context.Context, name, versionPrefix, arch, family string) (*spec.LayerManifest, error)

	// ResolveFormation returns the Formation for name@version.
	// Returns ErrNotFound if the formation does not exist.
	ResolveFormation(ctx context.Context, nameVersion, arch string) (*spec.Formation, error)

	// GetBaseCapabilities returns the cached BaseCapabilities for amiID.
	// Returns ErrNotFound if the AMI has not been probed.
	GetBaseCapabilities(ctx context.Context, amiID string) (*spec.BaseCapabilities, error)

	// StoreBaseCapabilities writes a BaseCapabilities record to the probe
	// cache. Called by the probe pipeline after a successful probe.
	StoreBaseCapabilities(ctx context.Context, caps *spec.BaseCapabilities) error

	// ListLayers returns all LayerManifests that match the given name,
	// filtered by arch and family. An empty name returns all layers.
	// Results are ordered by version descending (newest first).
	ListLayers(ctx context.Context, name, arch, family string) ([]*spec.LayerManifest, error)
}

// ErrNotFound is returned when a layer, formation, or capability set does
// not exist in the registry.
type ErrNotFound struct {
	Kind string // "layer", "formation", "capabilities"
	Key  string // the lookup key
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("registry: %s %q not found", e.Kind, e.Key)
}

// IsNotFound reports whether err is an ErrNotFound.
func IsNotFound(err error) bool {
	_, ok := err.(*ErrNotFound)
	return ok
}

// MemoryStore is an in-memory Client implementation for testing and
// local development. All operations are performed on Go maps.
type MemoryStore struct {
	layers       []*spec.LayerManifest
	formations   map[string]*spec.Formation        // key: "name@version"
	capabilities map[string]*spec.BaseCapabilities // key: AMI ID
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		formations:   make(map[string]*spec.Formation),
		capabilities: make(map[string]*spec.BaseCapabilities),
	}
}

// AddLayer registers a LayerManifest in the store. Used in tests to seed
// the catalog.
func (s *MemoryStore) AddLayer(m *spec.LayerManifest) {
	s.layers = append(s.layers, m)
}

// AddFormation registers a Formation in the store.
func (s *MemoryStore) AddFormation(f *spec.Formation) {
	s.formations[f.Name+"@"+f.Version] = f
}

// ResolveLayer returns the highest-versioned layer matching name, arch,
// family, and versionPrefix.
func (s *MemoryStore) ResolveLayer(_ context.Context, name, versionPrefix, arch, family string) (*spec.LayerManifest, error) {
	var best *spec.LayerManifest
	for _, m := range s.layers {
		if m.Name != name {
			continue
		}
		if arch != "" && m.Arch != arch {
			continue
		}
		if family != "" && m.Family != family {
			continue
		}
		if versionPrefix != "" && !versionMatches(m.Version, versionPrefix) {
			continue
		}
		if best == nil || versionGT(m.Version, best.Version) {
			best = m
		}
	}
	if best == nil {
		key := layerKey(name, versionPrefix, arch, family)
		return nil, &ErrNotFound{Kind: "layer", Key: key}
	}
	return best, nil
}

// ResolveFormation returns the Formation for nameVersion (e.g. "cuda-python-ml@2024.03").
func (s *MemoryStore) ResolveFormation(_ context.Context, nameVersion, _ string) (*spec.Formation, error) {
	f, ok := s.formations[nameVersion]
	if !ok {
		return nil, &ErrNotFound{Kind: "formation", Key: nameVersion}
	}
	return f, nil
}

// GetBaseCapabilities returns the cached BaseCapabilities for amiID.
func (s *MemoryStore) GetBaseCapabilities(_ context.Context, amiID string) (*spec.BaseCapabilities, error) {
	caps, ok := s.capabilities[amiID]
	if !ok {
		return nil, &ErrNotFound{Kind: "capabilities", Key: amiID}
	}
	return caps, nil
}

// StoreBaseCapabilities adds or replaces the BaseCapabilities for an AMI.
func (s *MemoryStore) StoreBaseCapabilities(_ context.Context, caps *spec.BaseCapabilities) error {
	s.capabilities[caps.AMIID] = caps
	return nil
}

// ListLayers returns all layers matching the filter criteria, newest-first.
func (s *MemoryStore) ListLayers(_ context.Context, name, arch, family string) ([]*spec.LayerManifest, error) {
	var result []*spec.LayerManifest
	for _, m := range s.layers {
		if name != "" && m.Name != name {
			continue
		}
		if arch != "" && m.Arch != arch {
			continue
		}
		if family != "" && m.Family != family {
			continue
		}
		result = append(result, m)
	}
	// Sort newest-first using the same numeric comparator as spec.
	sortManifestsByVersionDesc(result)
	return result, nil
}

// versionMatches reports whether version starts with the given prefix,
// treating each component as a numeric segment.
// "12.3" matches "12.3.0", "12.3.1", "12.3.2", etc.
// "12" matches "12.3", "12.4", etc.
func versionMatches(version, prefix string) bool {
	if prefix == "" {
		return true
	}
	vParts := strings.Split(version, ".")
	pParts := strings.Split(prefix, ".")
	if len(pParts) > len(vParts) {
		return false
	}
	for i, p := range pParts {
		if vParts[i] != p {
			return false
		}
	}
	return true
}

// versionGT reports whether a > b using numeric segment comparison.
func versionGT(a, b string) bool {
	return compareSegments(a, b) > 0
}

// compareSegments compares two dotted version strings numerically.
func compareSegments(a, b string) int {
	aParts := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bParts := strings.Split(strings.TrimPrefix(b, "v"), ".")
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := range n {
		var av, bv int
		if i < len(aParts) {
			av = parseSegment(aParts[i])
		}
		if i < len(bParts) {
			bv = parseSegment(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// parseSegment converts a version segment string to int, returning 0 on failure.
func parseSegment(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// sortManifestsByVersionDesc sorts manifests in-place, newest version first.
func sortManifestsByVersionDesc(manifests []*spec.LayerManifest) {
	for i := 1; i < len(manifests); i++ {
		for j := i; j > 0 && versionGT(manifests[j].Version, manifests[j-1].Version); j-- {
			manifests[j], manifests[j-1] = manifests[j-1], manifests[j]
		}
	}
}

// layerKey constructs a display key for error messages.
func layerKey(name, version, arch, family string) string {
	key := name
	if version != "" {
		key += "@" + version
	}
	if arch != "" || family != "" {
		key += " (" + arch + "/" + family + ")"
	}
	return key
}
