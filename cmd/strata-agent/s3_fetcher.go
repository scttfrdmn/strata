package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/strata/spec"
)

const defaultCacheDir = "/strata/cache"

// s3LayerFetcher downloads squashfs layers from S3 to the local layer cache.
type s3LayerFetcher struct {
	s3       s3GetAPI
	cacheDir string
}

// newS3LayerFetcher creates an s3LayerFetcher backed by a real AWS S3 client.
func newS3LayerFetcher() *s3LayerFetcher {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return &s3LayerFetcher{cacheDir: defaultCacheDir}
	}
	// All Strata registry resources live in us-east-1. Fall back when the
	// region cannot be resolved from the environment (e.g. early boot).
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &s3LayerFetcher{
		s3:       s3.NewFromConfig(cfg),
		cacheDir: defaultCacheDir,
	}
}

// newS3LayerFetcherWithAPI constructs an s3LayerFetcher with an injected API
// and cache directory — used by tests.
func newS3LayerFetcherWithAPI(api s3GetAPI, cacheDir string) *s3LayerFetcher {
	return &s3LayerFetcher{s3: api, cacheDir: cacheDir}
}

// Fetch downloads layer to the local cache if not already present and
// returns the local cache path. The agent verifies the SHA256 after Fetch returns.
func (f *s3LayerFetcher) Fetch(ctx context.Context, layer spec.ResolvedLayer) (string, error) {
	cachePath := filepath.Join(f.cacheDir, layer.SHA256+".sqfs")

	// 1. Cache hit: return immediately.
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	// 2. Parse S3 URI from layer.Source.
	bucket, key, ok := parseS3URI(layer.Source)
	if !ok {
		return "", fmt.Errorf("s3LayerFetcher: invalid source URI %q", layer.Source)
	}
	if f.s3 == nil {
		return "", fmt.Errorf("s3LayerFetcher: S3 client unavailable")
	}

	// 3. Fetch from S3.
	out, err := f.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return "", fmt.Errorf("s3LayerFetcher: fetching %q: %w", layer.Source, err)
	}
	defer out.Body.Close() //nolint:errcheck

	// 4. Write atomically: temp file in cacheDir then rename.
	if err := os.MkdirAll(f.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("s3LayerFetcher: creating cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(f.cacheDir, "*.sqfs.tmp")
	if err != nil {
		return "", fmt.Errorf("s3LayerFetcher: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, out.Body); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("s3LayerFetcher: writing layer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("s3LayerFetcher: closing temp file: %w", err)
	}

	// 5. Atomic rename (same filesystem on Linux).
	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("s3LayerFetcher: renaming to cache path: %w", err)
	}
	return cachePath, nil
}
