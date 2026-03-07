package registry

import (
	"context"
	"errors"

	"github.com/scttfrdmn/strata/spec"
)

// errS3NotImplemented is returned by all S3Client methods until the real
// S3 backend is wired up.
var errS3NotImplemented = errors.New("s3 registry: not yet implemented")

// S3Client implements registry.Client against an S3-backed Strata registry.
//
// The registry layout is:
//
//	s3://<bucket>/layers/<family>/<arch>/<name>/<version>/manifest.yaml
//	s3://<bucket>/formations/<name>/<version>/manifest.yaml
//	s3://<bucket>/probes/<ami-id>/capabilities.yaml
//	s3://<bucket>/index/layers.yaml
type S3Client struct {
	// BucketURL is the S3 bucket URL, e.g. "s3://strata-layers".
	BucketURL string
}

// NewS3Client creates an S3Client for the given bucket URL.
func NewS3Client(bucketURL string) *S3Client {
	return &S3Client{BucketURL: bucketURL}
}

// ResolveLayer looks up the best-matching layer manifest.
// TODO: list s3://<bucket>/layers/<family>/<arch>/<name>/ with version prefix
// matching, sort by version descending, fetch manifest.yaml for the best match.
func (c *S3Client) ResolveLayer(_ context.Context, _, _, _, _ string) (*spec.LayerManifest, error) {
	return nil, errS3NotImplemented
}

// ResolveFormation fetches a formation manifest.
// TODO: parse nameVersion as "name@version", fetch
// s3://<bucket>/formations/<name>/<version>/manifest.yaml.
func (c *S3Client) ResolveFormation(_ context.Context, _, _ string) (*spec.Formation, error) {
	return nil, errS3NotImplemented
}

// GetBaseCapabilities looks up the probe cache.
// TODO: fetch s3://<bucket>/probes/<amiID>/capabilities.yaml.
func (c *S3Client) GetBaseCapabilities(_ context.Context, _ string) (*spec.BaseCapabilities, error) {
	return nil, errS3NotImplemented
}

// StoreBaseCapabilities writes capabilities to the probe cache.
// TODO: put YAML to s3://<bucket>/probes/<caps.AMIID>/capabilities.yaml.
func (c *S3Client) StoreBaseCapabilities(_ context.Context, _ *spec.BaseCapabilities) error {
	return errS3NotImplemented
}

// ListLayers returns all layers in the index matching the given filters.
// TODO: fetch s3://<bucket>/index/layers.yaml and filter by name/arch/family.
func (c *S3Client) ListLayers(_ context.Context, _, _, _ string) ([]*spec.LayerManifest, error) {
	return nil, errS3NotImplemented
}
