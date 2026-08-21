package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LayerCacheExt is the file extension of a cached layer squashfs image.
const LayerCacheExt = ".sqfs"

// layerDigestLen is the length of a hex-encoded SHA256.
const layerDigestLen = 2 * sha256.Size

// ValidateLayerDigest reports whether digest is a well-formed layer digest:
// exactly 64 lowercase hex characters.
//
// Lowercase is required rather than merely accepted. Every digest producer in
// this module goes through hex.EncodeToString, which emits lowercase, and every
// consumer compares raw strings with !=, so an uppercase digest can never match
// a file this module hashed. Accepting one would only create a second cache
// entry for the same bytes that is guaranteed to fail verification.
//
// An empty digest is an error, not a permitted "unverified" state: a layer with
// no digest cannot be checked against anything, and the caller that is about to
// read its bytes has no way to tell content from substituted content.
func ValidateLayerDigest(digest string) error {
	if digest == "" {
		return fmt.Errorf("spec: layer digest is empty: a layer with no sha256 cannot be verified")
	}
	if len(digest) != layerDigestLen {
		return fmt.Errorf("spec: layer digest %q is %d characters, want %d lowercase hex",
			digest, len(digest), layerDigestLen)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return fmt.Errorf("spec: layer digest %q has %q at offset %d, want %d lowercase hex",
			digest, string(c), i, layerDigestLen)
	}
	return nil
}

// LayerCachePath returns the path of the cached squashfs for a layer with the
// given digest, or an error if the digest is not well-formed.
//
// The validation is the point of the function. filepath.Join calls Clean, which
// resolves ".." rather than rejecting it, so joining an unvalidated digest can
// name a file outside cacheDir. Every site that builds a layer cache path must
// go through here so that the filename is always the digest and nothing else.
func LayerCachePath(cacheDir, digest string) (string, error) {
	if err := ValidateLayerDigest(digest); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, digest+LayerCacheExt), nil
}

// FileDigest returns the lowercase hex SHA256 of the file at path.
func FileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DigestMismatchError reports a file whose contents do not hash to the digest
// that was declared for it.
type DigestMismatchError struct {
	Path string // file that was hashed
	Want string // declared digest
	Got  string // digest of the bytes on disk
}

func (e *DigestMismatchError) Error() string {
	return fmt.Sprintf("spec: %s: sha256 mismatch: declared=%s actual=%s", e.Path, e.Want, e.Got)
}

// VerifyFileDigest hashes the file at path and compares it to digest, which
// must itself be well-formed. It returns a *DigestMismatchError when the bytes
// on disk are not the bytes the digest names.
func VerifyFileDigest(path, digest string) error {
	if err := ValidateLayerDigest(digest); err != nil {
		return err
	}
	actual, err := FileDigest(path)
	if err != nil {
		return err
	}
	if actual != digest {
		return &DigestMismatchError{Path: path, Want: digest, Got: actual}
	}
	return nil
}
