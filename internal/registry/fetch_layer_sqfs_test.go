package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// These tests observe the #57/#82 tightening in FetchLayerSqfs — that a cache
// hit is re-hashed against the declared digest and a fresh download is verified
// before it is committed — which the existing localclient test cannot: it derives
// the declared digest from the very file it verifies, so it passes whether or not
// the check exists. The s3client path had no test at all (#84). The shared move is
// to make the declared digest and the bytes *disagree* and require a refusal.

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// plantCacheFile writes content at the cache path a given declared digest names,
// so the file exists (a cache hit) but its bytes are whatever content is.
func plantCacheFile(t *testing.T, cacheDir, declaredDigest string, content []byte) {
	t.Helper()
	p, err := spec.LayerCachePath(cacheDir, declaredDigest)
	if err != nil {
		t.Fatalf("LayerCachePath: %v", err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("planting cache file: %v", err)
	}
}

func TestS3ClientFetchLayerSqfs_VerifiesHonestDownload(t *testing.T) {
	content := []byte("honest squashfs bytes")
	digest := sha256hex(content)
	mock := newMockS3()
	mock.objects["layers/x/layer.sqfs"] = content
	c := newS3ClientWithAPI("bucket", mock)
	m := &spec.LayerManifest{ID: "x", SHA256: digest, Source: "s3://bucket/layers/x/layer.sqfs"}

	path, err := c.FetchLayerSqfs(context.Background(), m, t.TempDir())
	if err != nil {
		t.Fatalf("honest download rejected: %v", err)
	}
	if err := spec.VerifyFileDigest(path, digest); err != nil {
		t.Errorf("cached file does not hash to the declared digest: %v", err)
	}
}

func TestS3ClientFetchLayerSqfs_RejectsWrongDigestOnDownload(t *testing.T) {
	content := []byte("honest squashfs bytes")
	mock := newMockS3()
	mock.objects["layers/x/layer.sqfs"] = content
	c := newS3ClientWithAPI("bucket", mock)
	// Declared digest is of *different* bytes than S3 serves.
	m := &spec.LayerManifest{ID: "x", SHA256: sha256hex([]byte("other bytes")), Source: "s3://bucket/layers/x/layer.sqfs"}

	if _, err := c.FetchLayerSqfs(context.Background(), m, t.TempDir()); err == nil {
		t.Fatal("accepted a download whose bytes do not match the declared digest (#84)")
	}
}

func TestS3ClientFetchLayerSqfs_RejectsTamperedCacheHit(t *testing.T) {
	honest := []byte("honest squashfs bytes")
	digest := sha256hex(honest)
	cacheDir := t.TempDir()
	// A cache hit at the digest-named path, but the bytes there are tampered — the
	// filename is not evidence of the contents.
	plantCacheFile(t, cacheDir, digest, []byte("tampered"))

	c := newS3ClientWithAPI("bucket", newMockS3()) // never reached: the cache-hit branch fires first
	m := &spec.LayerManifest{ID: "x", SHA256: digest, Source: "s3://bucket/layers/x/layer.sqfs"}

	if _, err := c.FetchLayerSqfs(context.Background(), m, cacheDir); err == nil {
		t.Fatal("returned a cache hit whose bytes do not match the declared digest (#84)")
	}
}

func TestLocalClientFetchLayerSqfs_RejectsTamperedCacheHit(t *testing.T) {
	honest := []byte("honest squashfs bytes")
	digest := sha256hex(honest)
	cacheDir := t.TempDir()
	plantCacheFile(t, cacheDir, digest, []byte("tampered"))

	c, err := NewLocalClient("file://" + t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalClient: %v", err)
	}
	m := &spec.LayerManifest{ID: "x", SHA256: digest}

	if _, err := c.FetchLayerSqfs(context.Background(), m, cacheDir); err == nil {
		t.Fatal("returned a tampered cache hit; the existing happy-path test cannot catch this (#84)")
	}
}
