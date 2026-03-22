package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

// S3Client implements registry.Client against an S3-backed Strata registry.
//
// The registry layout is:
//
//	s3://<bucket>/layers/<abi>/<arch>/<name>/<version>/manifest.yaml
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
	// Fall back to us-east-1 when no region is resolved from env/config/IMDS.
	// All Strata registry resources live in us-east-1.
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
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
// abi, and version prefix match the request.
//
// It reads the flat index at index/layers.yaml (which contains complete
// manifests including Sigstore signing fields) and selects the best-matching
// version using versionMatches + compareSegments.
func (c *S3Client) ResolveLayer(ctx context.Context, name, versionPrefix, arch, abi string) (*spec.LayerManifest, error) {
	var idx LayerIndex
	if err := c.getYAML(ctx, "index/layers.yaml", &idx); err != nil {
		return nil, err
	}

	var best *spec.LayerManifest
	for _, m := range idx.Layers {
		if m.Name != name {
			continue
		}
		if arch != "" && m.Arch != arch {
			continue
		}
		if abi != "" && m.ABI != abi {
			continue
		}
		if versionPrefix != "" && !versionMatches(m.Version, versionPrefix) {
			continue
		}
		if best == nil || compareSegments(m.Version, best.Version) > 0 {
			cp := *m
			best = &cp
		}
	}
	if best == nil {
		return nil, &ErrNotFound{Kind: "layer", Key: layerKey(name, versionPrefix, arch, abi)}
	}
	return best, nil
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
// name, arch, and abi (any empty filter matches all). Results are sorted
// newest-first.
func (c *S3Client) ListLayers(ctx context.Context, name, arch, abi string) ([]*spec.LayerManifest, error) {
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
		if abi != "" && m.ABI != abi {
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
// under layers/<abi>/<arch>/<name>/<version>/, then upserts the manifest
// into index/layers.yaml.
func (c *S3Client) PushLayer(ctx context.Context, manifest *spec.LayerManifest, sqfsPath string, bundleJSON []byte) error {
	prefix := fmt.Sprintf("layers/%s/%s/%s/%s/", manifest.ABI, manifest.Arch, manifest.Name, manifest.Version)

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

	// Set Source and Bundle on the manifest before marshaling.
	manifest.Source = "s3://" + c.bucket + "/" + prefix + "layer.sqfs"
	manifest.Bundle = "s3://" + c.bucket + "/" + prefix + "bundle.json"

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

// FetchLayerSqfs downloads the squashfs file for manifest to cacheDir.
// If cacheDir already contains a file named <sha256>.sqfs it is returned
// immediately without re-downloading. The downloaded file is verified against
// manifest.SHA256 before it is committed to the cache.
func (c *S3Client) FetchLayerSqfs(ctx context.Context, manifest *spec.LayerManifest, cacheDir string) (string, error) {
	cachePath := filepath.Join(cacheDir, manifest.SHA256+".sqfs")
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("registry: creating cache dir: %w", err)
	}

	bucket, key, ok := parseObjectURI(manifest.Source)
	if !ok {
		return "", fmt.Errorf("registry: invalid layer source URI %q for %q", manifest.Source, manifest.ID)
	}

	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("registry: fetching squashfs for %q: %w", manifest.ID, err)
	}
	defer out.Body.Close() //nolint:errcheck

	tmp, err := os.CreateTemp(cacheDir, "*.sqfs.tmp")
	if err != nil {
		return "", fmt.Errorf("registry: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, out.Body); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: writing squashfs for %q: %w", manifest.ID, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: closing squashfs temp file: %w", err)
	}

	actual, err := hexSHA256File(tmpPath)
	if err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: hashing squashfs for %q: %w", manifest.ID, err)
	}
	if actual != manifest.SHA256 {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: squashfs SHA256 mismatch for %q: manifest=%q actual=%q",
			manifest.ID, manifest.SHA256, actual)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: caching squashfs for %q: %w", manifest.ID, err)
	}
	return cachePath, nil
}

// parseObjectURI splits an "s3://<bucket>/<key>" URI into bucket and key.
// Returns (bucket, key, true) on success, ("", "", false) otherwise.
func parseObjectURI(uri string) (bucket, key string, ok bool) {
	rest, found := strings.CutPrefix(uri, "s3://")
	if !found || rest == "" {
		return "", "", false
	}
	idx := strings.IndexByte(rest, '/')
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// SHA256HexFile returns the hex-encoded SHA256 of the named file.
// It is exported for use by commands that build or freeze layers.
func SHA256HexFile(path string) (string, error) {
	return hexSHA256File(path)
}

// hexSHA256File returns the hex-encoded SHA256 of the named file.
func hexSHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RebuildIndex scans all layer manifests in the registry and rewrites
// index/layers.yaml. Use after batch builds or to repair index inconsistency.
func (c *S3Client) RebuildIndex(ctx context.Context) error {
	// List all ABI prefixes: layers/<abi>/
	familyPrefixes, err := c.listCommonPrefixes(ctx, "layers/")
	if err != nil {
		return fmt.Errorf("registry: listing abi prefixes: %w", err)
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
					// Derive Bundle URI from the known layout if not already set.
					if m.Bundle == "" {
						m.Bundle = "s3://" + c.bucket + "/" + versionPrefix + "bundle.json"
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

// LockfileRecord pairs an S3 key with its parsed LockFile content.
type LockfileRecord struct {
	Key      string         // S3 key, e.g. "locks/abc123.yaml"
	LockFile *spec.LockFile // parsed content
}

// DeleteLayer removes the three S3 objects for a layer (manifest.yaml,
// layer.sqfs, bundle.json) using a single DeleteObjects call.
// Tolerates already-absent objects — S3 DeleteObjects is idempotent.
func (c *S3Client) DeleteLayer(ctx context.Context, manifest *spec.LayerManifest) error {
	prefix := fmt.Sprintf("layers/%s/%s/%s/%s/",
		manifest.ABI, manifest.Arch, manifest.Name, manifest.Version)
	keys := []types.ObjectIdentifier{
		{Key: aws.String(prefix + "manifest.yaml")},
		{Key: aws.String(prefix + "layer.sqfs")},
		{Key: aws.String(prefix + "bundle.json")},
	}
	_, err := c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(c.bucket),
		Delete: &types.Delete{Objects: keys, Quiet: aws.Bool(true)},
	})
	if err != nil {
		return fmt.Errorf("registry: deleting layer %s/%s: %w", manifest.Name, manifest.Version, err)
	}
	return nil
}

// ListLockfiles fetches every lockfile stored under locks/ and returns them
// as LockfileRecords. Fetches are issued in parallel. Returns nil slice (not
// an error) when no lockfiles exist.
func (c *S3Client) ListLockfiles(ctx context.Context) ([]LockfileRecord, error) {
	// Collect all keys under locks/.
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String("locks/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("registry: listing lockfiles: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil && strings.HasSuffix(*obj.Key, ".yaml") {
				keys = append(keys, *obj.Key)
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}

	// Fetch all lockfiles in parallel.
	results := make([]LockfileRecord, len(keys))
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			var lf spec.LockFile
			if err := c.getYAML(ctx, key, &lf); err != nil {
				errs[i] = fmt.Errorf("registry: fetching lockfile %s: %w", key, err)
				return
			}
			results[i] = LockfileRecord{Key: key, LockFile: &lf}
		}(i, key)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// PutLockfile stores the lockfile in S3 under locks/<environmentID>.yaml and
// returns its S3 URI (e.g. "s3://strata-registry/locks/abc123.yaml").
//
// The returned URI can be set as EC2 instance tag strata:lockfile-s3-uri;
// strata-agent reads the lockfile from S3 at boot using that tag.
func (c *S3Client) PutLockfile(ctx context.Context, lockfile *spec.LockFile) (string, error) {
	data, err := yaml.Marshal(lockfile)
	if err != nil {
		return "", fmt.Errorf("registry: marshalling lockfile: %w", err)
	}
	key := "locks/" + lockfile.EnvironmentID() + ".yaml"
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/yaml"),
	})
	if err != nil {
		return "", fmt.Errorf("registry: uploading lockfile: %w", err)
	}
	return "s3://" + c.bucket + "/" + key, nil
}
