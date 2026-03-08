package registry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// s3API is the subset of the S3 client used by S3Client. Defined as an
// interface to allow mock injection in tests.
type s3API interface {
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Client implements registry.Client against an S3-backed Strata registry.
//
// The registry layout is:
//
//	s3://<bucket>/layers/<family>/<arch>/<name>/<version>/manifest.yaml
//	s3://<bucket>/formations/<name>/<version>/manifest.yaml
//	s3://<bucket>/probes/<ami-id>/capabilities.yaml
//	s3://<bucket>/index/layers.yaml
type S3Client struct {
	bucket string
	s3     s3API
}

// NewS3Client creates an S3Client for the given bucket URL (e.g.
// "s3://strata-layers"). AWS credentials are loaded via the default
// provider chain: environment variables → EC2 instance profile →
// ~/.aws/credentials.
func NewS3Client(bucketURL string) (*S3Client, error) {
	bucket, ok := parseBucketURL(bucketURL)
	if !ok {
		return nil, fmt.Errorf("registry: invalid S3 URL %q (expected s3://<bucket>)", bucketURL)
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("registry: loading AWS config: %w", err)
	}
	return &S3Client{bucket: bucket, s3: s3.NewFromConfig(cfg)}, nil
}

// newS3ClientWithAPI constructs an S3Client with a pre-built API — used by
// tests to inject a mock without real AWS credentials.
func newS3ClientWithAPI(bucket string, api s3API) *S3Client {
	return &S3Client{bucket: bucket, s3: api}
}

// parseBucketURL extracts the bucket name from an "s3://<bucket>" URL.
func parseBucketURL(u string) (string, bool) {
	rest, ok := strings.CutPrefix(u, "s3://")
	if !ok || rest == "" {
		return "", false
	}
	// Strip any trailing path component — we only use the bucket name.
	bucket := strings.SplitN(rest, "/", 2)[0]
	if bucket == "" {
		return "", false
	}
	return bucket, true
}

// ResolveLayer returns the highest-versioned layer manifest whose name, arch,
// family, and version prefix match the request.
//
// It lists s3://<bucket>/layers/<family>/<arch>/<name>/ using the delimiter
// "/" to get one common-prefix per version directory, then applies
// versionMatches + compareSegments to select the best, and fetches its
// manifest.yaml.
func (c *S3Client) ResolveLayer(ctx context.Context, name, versionPrefix, arch, family string) (*spec.LayerManifest, error) {
	prefix := fmt.Sprintf("layers/%s/%s/%s/", family, arch, name)
	versions, err := c.listCommonPrefixes(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, &ErrNotFound{Kind: "layer", Key: layerKey(name, versionPrefix, arch, family)}
	}

	var bestVersion string
	for _, vdir := range versions {
		// vdir is like "layers/rhel/x86_64/python/3.11.9/"
		ver := extractLastSegment(strings.TrimSuffix(vdir, "/"))
		if versionPrefix != "" && !versionMatches(ver, versionPrefix) {
			continue
		}
		if bestVersion == "" || compareSegments(ver, bestVersion) > 0 {
			bestVersion = ver
		}
	}
	if bestVersion == "" {
		return nil, &ErrNotFound{Kind: "layer", Key: layerKey(name, versionPrefix, arch, family)}
	}

	key := fmt.Sprintf("layers/%s/%s/%s/%s/manifest.yaml", family, arch, name, bestVersion)
	var m spec.LayerManifest
	if err := c.getYAML(ctx, key, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ResolveFormation fetches the formation manifest for nameVersion
// (e.g. "cuda-python-ml@2024.03").
func (c *S3Client) ResolveFormation(ctx context.Context, nameVersion, _ string) (*spec.Formation, error) {
	parts := strings.SplitN(nameVersion, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("registry: invalid formation ref %q (expected name@version)", nameVersion)
	}
	key := fmt.Sprintf("formations/%s/%s/manifest.yaml", parts[0], parts[1])
	var f spec.Formation
	if err := c.getYAML(ctx, key, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// GetBaseCapabilities returns the cached BaseCapabilities for amiID.
func (c *S3Client) GetBaseCapabilities(ctx context.Context, amiID string) (*spec.BaseCapabilities, error) {
	key := fmt.Sprintf("probes/%s/capabilities.yaml", amiID)
	var caps spec.BaseCapabilities
	if err := c.getYAML(ctx, key, &caps); err != nil {
		return nil, err
	}
	return &caps, nil
}

// StoreBaseCapabilities writes a BaseCapabilities record to the probe cache.
func (c *S3Client) StoreBaseCapabilities(ctx context.Context, caps *spec.BaseCapabilities) error {
	data, err := yaml.Marshal(caps)
	if err != nil {
		return fmt.Errorf("registry: marshaling capabilities: %w", err)
	}
	key := fmt.Sprintf("probes/%s/capabilities.yaml", caps.AMIID)
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return fmt.Errorf("registry: storing capabilities for %s: %w", caps.AMIID, err)
	}
	return nil
}

// ListLayers fetches the flat layer index and returns all manifests matching
// name, arch, and family (any empty filter matches all). Results are sorted
// newest-first.
func (c *S3Client) ListLayers(ctx context.Context, name, arch, family string) ([]*spec.LayerManifest, error) {
	var idx LayerIndex
	if err := c.getYAML(ctx, "index/layers.yaml", &idx); err != nil {
		return nil, err
	}

	var result []*spec.LayerManifest
	for _, m := range idx.Layers {
		if name != "" && m.Name != name {
			continue
		}
		if arch != "" && m.Arch != arch {
			continue
		}
		if family != "" && m.Family != family {
			continue
		}
		cp := *m
		result = append(result, &cp)
	}
	sortManifestsByVersionDesc(result)
	return result, nil
}

// LayerIndex is the flat catalog index stored at index/layers.yaml in the
// registry bucket.
type LayerIndex struct {
	Layers []*spec.LayerManifest `yaml:"layers"`
}

// getYAML fetches an S3 object and unmarshals its body as YAML into dst.
// A missing key is mapped to *ErrNotFound.
func (c *S3Client) getYAML(ctx context.Context, key string, dst any) error {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return &ErrNotFound{Kind: kindFromKey(key), Key: key}
		}
		return fmt.Errorf("registry: fetching s3://%s/%s: %w", c.bucket, key, err)
	}
	defer out.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return fmt.Errorf("registry: reading s3://%s/%s: %w", c.bucket, key, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("registry: parsing s3://%s/%s: %w", c.bucket, key, err)
	}
	return nil
}

// listCommonPrefixes returns the common prefixes for objects under prefix
// using "/" as the delimiter. This gives the set of immediate subdirectories.
func (c *S3Client) listCommonPrefixes(ctx context.Context, prefix string) ([]string, error) {
	var prefixes []string
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket:    aws.String(c.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("registry: listing s3://%s/%s: %w", c.bucket, prefix, err)
		}
		for _, cp := range page.CommonPrefixes {
			if cp.Prefix != nil {
				prefixes = append(prefixes, *cp.Prefix)
			}
		}
	}
	return prefixes, nil
}

// extractLastSegment returns the last path component (after the final "/",
// treating a trailing "/" as already stripped before calling this).
func extractLastSegment(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

// kindFromKey infers the ErrNotFound.Kind from the S3 key path.
func kindFromKey(key string) string {
	switch {
	case strings.HasPrefix(key, "layers/"):
		return "layer"
	case strings.HasPrefix(key, "formations/"):
		return "formation"
	case strings.HasPrefix(key, "probes/"):
		return "capabilities"
	default:
		return "object"
	}
}

// PushLayer uploads layer.sqfs, manifest.yaml, and bundle.json to the registry
// under layers/<family>/<arch>/<name>/<version>/, then upserts the manifest
// into index/layers.yaml.
func (c *S3Client) PushLayer(ctx context.Context, manifest *spec.LayerManifest, sqfsPath string, bundleJSON []byte) error {
	prefix := fmt.Sprintf("layers/%s/%s/%s/%s/", manifest.Family, manifest.Arch, manifest.Name, manifest.Version)

	// Upload layer.sqfs.
	sqfsFile, err := os.Open(sqfsPath)
	if err != nil {
		return fmt.Errorf("registry: opening sqfs %q: %w", sqfsPath, err)
	}
	defer sqfsFile.Close() //nolint:errcheck

	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(prefix + "layer.sqfs"),
		Body:        sqfsFile,
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("registry: uploading layer.sqfs: %w", err)
	}

	// Set Source on the manifest before marshaling.
	manifest.Source = "s3://" + c.bucket + "/" + prefix + "layer.sqfs"

	// Upload manifest.yaml.
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("registry: marshaling manifest: %w", err)
	}
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(prefix + "manifest.yaml"),
		Body:        bytes.NewReader(manifestData),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return fmt.Errorf("registry: uploading manifest.yaml: %w", err)
	}

	// Upload bundle.json.
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(prefix + "bundle.json"),
		Body:        bytes.NewReader(bundleJSON),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("registry: uploading bundle.json: %w", err)
	}

	return c.upsertLayerIndex(ctx, manifest)
}

// upsertLayerIndex fetches the current index, replaces the entry with the
// same manifest.ID (or appends if new), and writes it back atomically.
func (c *S3Client) upsertLayerIndex(ctx context.Context, manifest *spec.LayerManifest) error {
	var idx LayerIndex
	err := c.getYAML(ctx, "index/layers.yaml", &idx)
	if err != nil && !IsNotFound(err) {
		return fmt.Errorf("registry: fetching layer index: %w", err)
	}

	replaced := false
	for i, m := range idx.Layers {
		if m.ID == manifest.ID {
			cp := *manifest
			idx.Layers[i] = &cp
			replaced = true
			break
		}
	}
	if !replaced {
		cp := *manifest
		idx.Layers = append(idx.Layers, &cp)
	}

	data, err := yaml.Marshal(&idx)
	if err != nil {
		return fmt.Errorf("registry: marshaling layer index: %w", err)
	}
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String("index/layers.yaml"),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return fmt.Errorf("registry: writing layer index: %w", err)
	}
	return nil
}

// RebuildIndex scans all layer manifests in the registry and rewrites
// index/layers.yaml. Use after batch builds or to repair index inconsistency.
func (c *S3Client) RebuildIndex(ctx context.Context) error {
	// List all family prefixes: layers/<family>/
	familyPrefixes, err := c.listCommonPrefixes(ctx, "layers/")
	if err != nil {
		return fmt.Errorf("registry: listing families: %w", err)
	}

	var manifests []*spec.LayerManifest
	for _, familyPrefix := range familyPrefixes {
		archPrefixes, err := c.listCommonPrefixes(ctx, familyPrefix)
		if err != nil {
			return fmt.Errorf("registry: listing archs under %q: %w", familyPrefix, err)
		}
		for _, archPrefix := range archPrefixes {
			namePrefixes, err := c.listCommonPrefixes(ctx, archPrefix)
			if err != nil {
				return fmt.Errorf("registry: listing names under %q: %w", archPrefix, err)
			}
			for _, namePrefix := range namePrefixes {
				versionPrefixes, err := c.listCommonPrefixes(ctx, namePrefix)
				if err != nil {
					return fmt.Errorf("registry: listing versions under %q: %w", namePrefix, err)
				}
				for _, versionPrefix := range versionPrefixes {
					key := versionPrefix + "manifest.yaml"
					var m spec.LayerManifest
					if err := c.getYAML(ctx, key, &m); err != nil {
						if IsNotFound(err) {
							continue
						}
						return fmt.Errorf("registry: fetching manifest %q: %w", key, err)
					}
					cp := m
					manifests = append(manifests, &cp)
				}
			}
		}
	}

	idx := LayerIndex{Layers: manifests}
	data, err := yaml.Marshal(&idx)
	if err != nil {
		return fmt.Errorf("registry: marshaling rebuilt index: %w", err)
	}
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String("index/layers.yaml"),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return fmt.Errorf("registry: writing rebuilt index: %w", err)
	}
	return nil
}
