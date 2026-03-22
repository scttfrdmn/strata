package registry

import (
	"context"
	"errors"

	"github.com/scttfrdmn/strata/spec"
)

// FederatedClient searches multiple registries in priority order.
//
// For resolution methods (ResolveLayer, ResolveFormation, GetBaseCapabilities,
// ListLayers), it tries each registry in order and returns the first
// non-ErrNotFound result. This allows a local or institutional registry to
// shadow layers in the public registry.
//
// ListLayers merges results from all registries, deduplicating by layer ID.
// Higher-priority registries win when the same ID appears in multiple registries.
//
// Write methods (StoreBaseCapabilities) write to the first (highest-priority)
// registry only.
type FederatedClient struct {
	clients []Client
}

// NewFederatedClient returns a FederatedClient that searches clients in order.
// The first client has highest priority.
func NewFederatedClient(clients []Client) *FederatedClient {
	return &FederatedClient{clients: clients}
}

// ResolveLayer tries each registry in priority order and returns the first match.
func (f *FederatedClient) ResolveLayer(ctx context.Context, name, versionPrefix, arch, abi string) (*spec.LayerManifest, error) {
	var lastErr error
	for _, c := range f.clients {
		m, err := c.ResolveLayer(ctx, name, versionPrefix, arch, abi)
		if err == nil {
			return m, nil
		}
		if IsNotFound(err) {
			lastErr = err
			continue
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ErrNotFound{Kind: "layer", Key: layerKey(name, versionPrefix, arch, abi)}
}

// ResolveFormation tries each registry in priority order and returns the first match.
func (f *FederatedClient) ResolveFormation(ctx context.Context, nameVersion, arch string) (*spec.Formation, error) {
	var lastErr error
	for _, c := range f.clients {
		fm, err := c.ResolveFormation(ctx, nameVersion, arch)
		if err == nil {
			return fm, nil
		}
		if IsNotFound(err) {
			lastErr = err
			continue
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ErrNotFound{Kind: "formation", Key: nameVersion}
}

// GetBaseCapabilities tries each registry in priority order and returns the first match.
func (f *FederatedClient) GetBaseCapabilities(ctx context.Context, amiID string) (*spec.BaseCapabilities, error) {
	var lastErr error
	for _, c := range f.clients {
		caps, err := c.GetBaseCapabilities(ctx, amiID)
		if err == nil {
			return caps, nil
		}
		if IsNotFound(err) {
			lastErr = err
			continue
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ErrNotFound{Kind: "capabilities", Key: amiID}
}

// StoreBaseCapabilities writes to the first (highest-priority) registry only.
func (f *FederatedClient) StoreBaseCapabilities(ctx context.Context, caps *spec.BaseCapabilities) error {
	if len(f.clients) == 0 {
		return errors.New("registry: federated client has no registries")
	}
	return f.clients[0].StoreBaseCapabilities(ctx, caps)
}

// ListLayers merges results from all registries, deduplicating by layer ID.
// Higher-priority registries win when the same ID appears in multiple registries.
// Results are sorted newest-first by version.
func (f *FederatedClient) ListLayers(ctx context.Context, name, arch, abi string) ([]*spec.LayerManifest, error) {
	seen := make(map[string]bool)
	var merged []*spec.LayerManifest

	for _, c := range f.clients {
		layers, err := c.ListLayers(ctx, name, arch, abi)
		if err != nil && !IsNotFound(err) {
			return nil, err
		}
		for _, m := range layers {
			if seen[m.ID] {
				continue // higher-priority registry already won
			}
			seen[m.ID] = true
			cp := *m
			merged = append(merged, &cp)
		}
	}

	sortManifestsByVersionDesc(merged)
	return merged, nil
}
