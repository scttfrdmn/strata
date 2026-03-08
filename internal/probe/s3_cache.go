package probe

import (
	"context"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// CapabilityStore is the subset of registry.Client used for probe caching.
// The real *registry.S3Client satisfies this interface.
type CapabilityStore interface {
	GetBaseCapabilities(ctx context.Context, amiID string) (*spec.BaseCapabilities, error)
	StoreBaseCapabilities(ctx context.Context, caps *spec.BaseCapabilities) error
}

// S3Cache implements Cache via a CapabilityStore. Cache entries are stored at
// s3://<bucket>/probes/<amiID>/capabilities.yaml (managed by registry.S3Client).
type S3Cache struct {
	store CapabilityStore
}

// NewS3Cache returns an S3Cache backed by the given CapabilityStore.
func NewS3Cache(store CapabilityStore) *S3Cache {
	return &S3Cache{store: store}
}

// Get returns the cached BaseCapabilities for amiID. Returns nil, false on a
// cache miss or any error — a missing entry is not treated as a fatal error.
func (c *S3Cache) Get(ctx context.Context, amiID string) (*spec.BaseCapabilities, bool) {
	caps, err := c.store.GetBaseCapabilities(ctx, amiID)
	if err != nil {
		// registry.IsNotFound is a normal cache miss; other errors are also
		// treated as misses so that probe execution proceeds.
		_ = registry.IsNotFound(err) // checked implicitly — all errors → miss
		return nil, false
	}
	return caps, true
}

// Set stores caps in the backing CapabilityStore.
func (c *S3Cache) Set(ctx context.Context, _ string, caps *spec.BaseCapabilities) error {
	return c.store.StoreBaseCapabilities(ctx, caps)
}
