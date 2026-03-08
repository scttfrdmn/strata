package build

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// GenerateContentManifest walks dir recursively and computes the SHA256 of
// each regular file. Paths in Files are relative to dir with a leading "/".
func GenerateContentManifest(dir, layerID string) (*ContentManifest, error) {
	files := make(map[string]string)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("content manifest: rel path: %w", err)
		}
		digest, err := sha256HexFile(path)
		if err != nil {
			return fmt.Errorf("content manifest: hashing %q: %w", path, err)
		}
		files["/"+filepath.ToSlash(rel)] = digest
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("content manifest: walking %q: %w", dir, err)
	}

	return &ContentManifest{
		LayerID: layerID,
		Files:   files,
	}, nil
}

// sha256HexFile returns the hex-encoded SHA256 digest of the file at path.
func sha256HexFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
