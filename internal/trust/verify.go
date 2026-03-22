package trust

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/scttfrdmn/strata/spec"
)

// VerifyLayer verifies a resolved layer before it is mounted.
// It performs two independent checks:
//
//  1. SHA256 of squashfsPath matches manifest.SHA256 (content integrity)
//  2. Cosign bundle in manifest.Bundle is valid for squashfsPath (authenticity)
//
// Both checks must pass. A failure in either means the layer is untrusted
// and must not be mounted. There is no partial success.
func VerifyLayer(ctx context.Context, manifest *spec.LayerManifest, squashfsPath string, v Verifier) error {
	// Step 1: content integrity — SHA256 of the on-disk file must match the manifest.
	if manifest.SHA256 == "" {
		return fmt.Errorf("layer %q has no SHA256 in manifest: cannot verify", manifest.ID)
	}
	actual, err := sha256File(squashfsPath)
	if err != nil {
		return fmt.Errorf("layer %q: hashing %q: %w", manifest.ID, squashfsPath, err)
	}
	if actual != manifest.SHA256 {
		return fmt.Errorf("layer %q: SHA256 mismatch: manifest=%q file=%q",
			manifest.ID, manifest.SHA256, actual)
	}

	// Step 2: authenticity — cosign bundle must validate against the file.
	if manifest.Bundle == "" {
		return fmt.Errorf("layer %q has no bundle path in manifest: unsigned layers will not mount", manifest.ID)
	}

	bundleData, err := os.ReadFile(manifest.Bundle)
	if err != nil {
		return fmt.Errorf("layer %q: reading bundle %q: %w", manifest.ID, manifest.Bundle, err)
	}

	bundle, err := ParseBundle(bundleData)
	if err != nil {
		return fmt.Errorf("layer %q: parsing bundle: %w", manifest.ID, err)
	}

	if !bundle.HasRekorEntry() {
		return fmt.Errorf("layer %q: bundle has no Rekor entry: unsigned layers will not mount", manifest.ID)
	}

	if err := v.Verify(ctx, squashfsPath, bundle); err != nil {
		return fmt.Errorf("layer %q: signature verification failed: %w", manifest.ID, err)
	}

	return nil
}

// VerifyLayers verifies all layers in a lockfile in parallel.
// Each layer must pass VerifyLayer. The first failure aborts all remaining
// verifications and returns an error naming the failing layer.
// squashfsDir is the directory where layer squashfs files are cached,
// named as "<layer.ID>.sqfs".
func VerifyLayers(ctx context.Context, lockfile *spec.LockFile, squashfsDir string, v Verifier) error {
	type result struct {
		layerID string
		err     error
	}

	// Use a child context so that returning on first error cancels any
	// still-running verification goroutines promptly.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan result, len(lockfile.Layers))
	for _, layer := range lockfile.Layers {
		layer := layer // capture
		go func() {
			// filepath.Join normalises the path, preventing a layer.ID
			// containing ".." sequences from escaping squashfsDir.
			path := filepath.Join(squashfsDir, layer.ID+".sqfs")
			err := VerifyLayer(ctx, &layer.LayerManifest, path, v)
			results <- result{layerID: layer.ID, err: err}
		}()
	}

	for range len(lockfile.Layers) {
		r := <-results
		if r.err != nil {
			return r.err
		}
	}
	return nil
}

// sha256File returns the hex-encoded SHA256 of the named file's contents.
func sha256File(path string) (string, error) {
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
