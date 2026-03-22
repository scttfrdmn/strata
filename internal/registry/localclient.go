package registry

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// LocalClient implements the registry Client interface against a local
// filesystem directory. The directory layout is identical to the S3 layout:
//
//	<root>/layers/<abi>/<arch>/<name>/<version>/manifest.yaml
//	<root>/layers/<abi>/<arch>/<name>/<version>/layer.sqfs
//	<root>/layers/<abi>/<arch>/<name>/<version>/bundle.json
//	<root>/formations/<name>/<version>/manifest.yaml
//	<root>/probes/<ami-id>/capabilities.yaml
//	<root>/index/layers.yaml
//	<root>/locks/<environmentID>.yaml
//
// LocalClient is safe for concurrent use. It implements both Client and the
// push/fetch methods from S3Client (PushLayer, RebuildIndex, FetchLayerSqfs,
// DeleteLayer, PutLockfile, ListLockfiles) so it can be used wherever S3Client
// is used, including build.PushRegistry.
type LocalClient struct {
	root string
	mu   sync.RWMutex // protects index writes
}

// NewLocalClient creates a LocalClient for the given URL.
// url may be a "file:///path" URL or a raw filesystem path.
// The root directory and its subdirectories are created if they do not exist.
func NewLocalClient(url string) (*LocalClient, error) {
	root, ok := parseLocalURL(url)
	if !ok {
		// Treat as a raw path if it does not start with "file://".
		root = url
	}
	if root == "" {
		return nil, fmt.Errorf("registry: empty path in local registry URL %q", url)
	}
	for _, sub := range []string{"layers", "formations", "probes", "index", "locks"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("registry: creating local registry dir %q: %w", sub, err)
		}
	}
	return &LocalClient{root: root}, nil
}

// parseLocalURL strips the "file://" prefix and returns (path, true).
// Returns ("", false) if the URL does not start with "file://".
func parseLocalURL(u string) (string, bool) {
	rest, ok := strings.CutPrefix(u, "file://")
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

// path returns the full filesystem path for a registry-relative key.
func (c *LocalClient) path(key string) string {
	return filepath.Join(c.root, filepath.FromSlash(key))
}

// readYAML reads the file at key relative to root and unmarshals it into dst.
// A missing file is mapped to *ErrNotFound.
func (c *LocalClient) readYAML(key string, dst any) error {
	p := c.path(key)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &ErrNotFound{Kind: kindFromKey(key), Key: key}
		}
		return fmt.Errorf("registry: reading %q: %w", p, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("registry: parsing %q: %w", p, err)
	}
	return nil
}

// writeYAML marshals src and writes it to key relative to root.
func (c *LocalClient) writeYAML(key string, src any) error {
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("registry: creating dir for %q: %w", p, err)
	}
	data, err := yaml.Marshal(src)
	if err != nil {
		return fmt.Errorf("registry: marshaling %q: %w", key, err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("registry: writing %q: %w", p, err)
	}
	return nil
}

// ResolveLayer returns the highest-versioned layer matching the request.
// It reads index/layers.yaml and applies the same selection logic as S3Client.
func (c *LocalClient) ResolveLayer(_ context.Context, name, versionPrefix, arch, abi string) (*spec.LayerManifest, error) {
	var idx LayerIndex
	if err := c.readYAML("index/layers.yaml", &idx); err != nil {
		if IsNotFound(err) {
			return nil, &ErrNotFound{Kind: "layer", Key: layerKey(name, versionPrefix, arch, abi)}
		}
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
func (c *LocalClient) ResolveFormation(_ context.Context, nameVersion, _ string) (*spec.Formation, error) {
	parts := strings.SplitN(nameVersion, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("registry: invalid formation ref %q (expected name@version)", nameVersion)
	}
	key := fmt.Sprintf("formations/%s/%s/manifest.yaml", parts[0], parts[1])
	var f spec.Formation
	if err := c.readYAML(key, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// GetBaseCapabilities returns the cached BaseCapabilities for amiID.
func (c *LocalClient) GetBaseCapabilities(_ context.Context, amiID string) (*spec.BaseCapabilities, error) {
	key := fmt.Sprintf("probes/%s/capabilities.yaml", amiID)
	var caps spec.BaseCapabilities
	if err := c.readYAML(key, &caps); err != nil {
		return nil, err
	}
	return &caps, nil
}

// StoreBaseCapabilities writes a BaseCapabilities record to the local probe cache.
func (c *LocalClient) StoreBaseCapabilities(_ context.Context, caps *spec.BaseCapabilities) error {
	key := fmt.Sprintf("probes/%s/capabilities.yaml", caps.AMIID)
	return c.writeYAML(key, caps)
}

// ListLayers fetches the flat layer index and returns all matching manifests,
// newest-first. An empty filter field matches all values.
func (c *LocalClient) ListLayers(_ context.Context, name, arch, abi string) ([]*spec.LayerManifest, error) {
	var idx LayerIndex
	if err := c.readYAML("index/layers.yaml", &idx); err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
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

// PushLayer writes layer.sqfs, manifest.yaml, and bundle.json to the local
// registry directory and upserts the manifest into index/layers.yaml.
// Implements build.PushRegistry.
func (c *LocalClient) PushLayer(_ context.Context, manifest *spec.LayerManifest, sqfsPath string, bundleJSON []byte) error {
	prefix := fmt.Sprintf("layers/%s/%s/%s/%s/", manifest.ABI, manifest.Arch, manifest.Name, manifest.Version)

	// Set Source and Bundle URIs before writing.
	manifest.Source = "file://" + c.root + "/" + strings.ReplaceAll(prefix, "\\", "/") + "layer.sqfs"
	manifest.Bundle = "file://" + c.root + "/" + strings.ReplaceAll(prefix, "\\", "/") + "bundle.json"

	// Copy layer.sqfs.
	sqfsDst := c.path(prefix + "layer.sqfs")
	if err := os.MkdirAll(filepath.Dir(sqfsDst), 0o755); err != nil {
		return fmt.Errorf("registry: creating layer dir: %w", err)
	}
	if err := copyFile(sqfsPath, sqfsDst); err != nil {
		return fmt.Errorf("registry: copying layer.sqfs: %w", err)
	}

	// Write manifest.yaml.
	if err := c.writeYAML(prefix+"manifest.yaml", manifest); err != nil {
		return fmt.Errorf("registry: writing manifest.yaml: %w", err)
	}

	// Write bundle.json.
	bundleDst := c.path(prefix + "bundle.json")
	if err := os.WriteFile(bundleDst, bundleJSON, 0o644); err != nil {
		return fmt.Errorf("registry: writing bundle.json: %w", err)
	}

	return c.upsertLayerIndex(manifest)
}

// upsertLayerIndex fetches the current index, replaces the entry with the
// same manifest.ID (or appends if new), and writes it back.
// Protected by a mutex to allow concurrent pushes.
func (c *LocalClient) upsertLayerIndex(manifest *spec.LayerManifest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var idx LayerIndex
	err := c.readYAML("index/layers.yaml", &idx)
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

	return c.writeYAML("index/layers.yaml", &idx)
}

// FetchLayerSqfs copies the squashfs file from the local registry to cacheDir.
// If cacheDir already contains <sha256>.sqfs it is returned immediately.
// The copied file is verified against manifest.SHA256 before being committed.
func (c *LocalClient) FetchLayerSqfs(_ context.Context, manifest *spec.LayerManifest, cacheDir string) (string, error) {
	cachePath := filepath.Join(cacheDir, manifest.SHA256+".sqfs")
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("registry: creating cache dir: %w", err)
	}

	// Resolve source path from the file:// URI or fall back to computed path.
	srcPath, ok := localPathFromURI(manifest.Source)
	if !ok {
		// Reconstruct from layout when Source is not a file:// URI.
		prefix := fmt.Sprintf("layers/%s/%s/%s/%s/", manifest.ABI, manifest.Arch, manifest.Name, manifest.Version)
		srcPath = c.path(prefix + "layer.sqfs")
	}

	tmp, err := os.CreateTemp(cacheDir, "*.sqfs.tmp")
	if err != nil {
		return "", fmt.Errorf("registry: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close() //nolint:errcheck

	if err := copyFile(srcPath, tmpPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", fmt.Errorf("registry: copying squashfs for %q: %w", manifest.ID, err)
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

// RebuildIndex walks the local layers/ tree and rewrites index/layers.yaml.
func (c *LocalClient) RebuildIndex(_ context.Context) error {
	var manifests []*spec.LayerManifest

	layersRoot := filepath.Join(c.root, "layers")
	err := filepath.WalkDir(layersRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "manifest.yaml" {
			var m spec.LayerManifest
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil // skip unreadable manifests
			}
			if unmarshalErr := yaml.Unmarshal(data, &m); unmarshalErr != nil {
				return nil // skip malformed manifests
			}
			// Derive Bundle path from well-known layout if not set.
			if m.Bundle == "" {
				dir := filepath.Dir(path)
				m.Bundle = "file://" + filepath.ToSlash(filepath.Join(dir, "bundle.json"))
			}
			cp := m
			manifests = append(manifests, &cp)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("registry: walking layers tree: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeYAML("index/layers.yaml", &LayerIndex{Layers: manifests})
}

// DeleteLayer removes the three local files for a layer
// (manifest.yaml, layer.sqfs, bundle.json). Tolerates already-absent files.
func (c *LocalClient) DeleteLayer(_ context.Context, manifest *spec.LayerManifest) error {
	prefix := fmt.Sprintf("layers/%s/%s/%s/%s/",
		manifest.ABI, manifest.Arch, manifest.Name, manifest.Version)
	for _, name := range []string{"manifest.yaml", "layer.sqfs", "bundle.json"} {
		p := c.path(prefix + name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("registry: deleting %s: %w", name, err)
		}
	}
	return nil
}

// PutLockfile writes the lockfile to locks/<environmentID>.yaml and returns
// the file:// URI.
func (c *LocalClient) PutLockfile(_ context.Context, lockfile *spec.LockFile) (string, error) {
	key := "locks/" + lockfile.EnvironmentID() + ".yaml"
	if err := c.writeYAML(key, lockfile); err != nil {
		return "", fmt.Errorf("registry: writing lockfile: %w", err)
	}
	return "file://" + c.root + "/" + key, nil
}

// ListLockfiles returns all lockfiles stored under locks/.
func (c *LocalClient) ListLockfiles(_ context.Context) ([]LockfileRecord, error) {
	locksDir := filepath.Join(c.root, "locks")
	entries, err := os.ReadDir(locksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("registry: listing lockfiles: %w", err)
	}

	var records []LockfileRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		key := "locks/" + entry.Name()
		var lf spec.LockFile
		if err := c.readYAML(key, &lf); err != nil {
			return nil, fmt.Errorf("registry: reading lockfile %s: %w", key, err)
		}
		records = append(records, LockfileRecord{Key: key, LockFile: &lf})
	}
	return records, nil
}

// copyFile copies src to dst, creating dst's parent directories as needed.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// localPathFromURI converts a file:// URI to a filesystem path.
// Returns ("", false) if the URI is not a file:// URI.
func localPathFromURI(uri string) (string, bool) {
	rest, ok := strings.CutPrefix(uri, "file://")
	if !ok || rest == "" {
		return "", false
	}
	return filepath.FromSlash(rest), true
}
