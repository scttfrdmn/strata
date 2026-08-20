// Package testregistry materializes a valid file:// Strata registry from
// repo-resident fixture data, so that resolution can reach stage 8 with no AWS
// credentials, no network, and no prior build.
//
// The embedded Tier 0 catalog is *recipes*: it carries no sha256, bundle, or
// rekor_entry, because nothing has been built. Resolution against it therefore
// dies in stage 7 (BUNDLE_MISSING) for every profile. This package is the
// offline substitute — a two-layer registry whose manifests carry a real
// sha256 over real bytes, a bundle that exists on disk, and a numeric Rekor
// entry that survives strconv.ParseInt.
//
// # Using it
//
// From a test:
//
//	root, client := testregistry.New(t)          // registry in t.TempDir()
//	profile := testregistry.WriteProfile(t, dir, testregistry.ProfileMinimal)
//
// From a shell (CI, or a human reproducing an offline resolve):
//
//	go run ./internal/testregistry/mkregistry /tmp/strata-fixture
//	STRATA_REGISTRY_URL=file:///tmp/strata-fixture strata resolve profile.yaml
//
// # What the fixture is not
//
// Two things about it are deliberately fake, and are labelled as such inside
// the files themselves:
//
//   - layer.sqfs is not a squashfs image. It is a short text blob. Its sha256
//     and size in the manifest are real, so anything that hashes or fetches a
//     layer works against it; anything that *mounts* one fails loudly rather
//     than appearing to succeed.
//   - bundle.json is not a valid Sigstore bundle. No signature over this
//     content exists. Its messageDigest is the real SHA-256 of layer.sqfs; every
//     other field is synthetic. It satisfies stage 7's presence check, which is
//     all stage 7 checks today. A test that needs real signature verification
//     needs a real key, and that is not this fixture's job.
//
// Extending it is adding files under testdata/registry — Materialize is
// data-driven and discovers whatever manifests are there. It verifies each
// committed sha256 against the committed bytes, so a fixture that drifts fails
// with a message naming the file rather than rotting silently.
package testregistry

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

//go:embed testdata/registry testdata/profiles
var fixture embed.FS

const (
	registryRoot = "testdata/registry"
	profilesRoot = "testdata/profiles"

	// ProfileMinimal resolves two layers via direct software refs.
	ProfileMinimal = "offline-minimal.yaml"

	// ProfileFormation resolves the same two layers via a formation ref,
	// exercising stage 2.
	ProfileFormation = "offline-formation.yaml"

	// FormationRef is the formation ProfileFormation refers to.
	FormationRef = "strata-fixture@2026.08"
)

// LayerIDs are the layer IDs in the fixture registry, in stage-6 dependency
// order: python provides what jupyterlab requires, so python mounts first.
var LayerIDs = []string{
	"python-3.13.2-linux-gnu-2.34-x86_64",
	"jupyterlab-4.2.0-linux-gnu-2.34-x86_64",
}

// Materialize writes the fixture registry into dir, which is created if it does
// not exist, and returns dir as an absolute path.
//
// Manifests are committed with empty source and bundle fields because those are
// absolute file:// URIs that depend on where the registry lands. Materialize
// fills them in, verifies each layer manifest's sha256 and size against the
// committed layer.sqfs bytes, and then builds index/layers.yaml with the same
// registry.LocalClient.RebuildIndex that a real local registry uses.
func Materialize(ctx context.Context, dir string) (string, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("testregistry: resolving %q: %w", dir, err)
	}
	if err := copyTree(registryRoot, root); err != nil {
		return "", err
	}
	if err := VerifyLayerDigests(root); err != nil {
		return "", err
	}
	if err := rewriteLayerManifests(root); err != nil {
		return "", err
	}
	if err := rewriteFormationManifests(root); err != nil {
		return "", err
	}

	client, err := registry.NewLocalClient(uriFor(root))
	if err != nil {
		return "", fmt.Errorf("testregistry: opening materialized registry: %w", err)
	}
	if err := client.RebuildIndex(ctx); err != nil {
		return "", fmt.Errorf("testregistry: building layer index: %w", err)
	}

	// A registry whose index does not list every fixture layer is not usable,
	// and the resolver would report it as a missing layer several stages later.
	for _, id := range LayerIDs {
		found, err := indexHas(client, id)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("testregistry: layer %q missing from index/layers.yaml after rebuild", id)
		}
	}
	return root, nil
}

// indexHas reports whether the rebuilt index lists a layer with the given ID.
func indexHas(client *registry.LocalClient, id string) (bool, error) {
	layers, err := client.ListLayers(context.Background(), "", "", "")
	if err != nil {
		return false, fmt.Errorf("testregistry: listing layers: %w", err)
	}
	for _, m := range layers {
		if m.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// copyTree writes every embedded file under src to the corresponding path
// under dst.
func copyTree(src, dst string) error {
	return fs.WalkDir(fixture, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if mkErr := os.MkdirAll(target, 0o755); mkErr != nil {
				return fmt.Errorf("testregistry: creating %q: %w", target, mkErr)
			}
			return nil
		}
		data, err := fixture.ReadFile(p)
		if err != nil {
			return fmt.Errorf("testregistry: reading embedded %q: %w", p, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("testregistry: writing %q: %w", target, err)
		}
		return nil
	})
}

// VerifyLayerDigests checks every layer manifest under a materialized registry
// root against the bytes next to it: the declared sha256 must hash the declared
// layer.sqfs, the declared size must match it, and bundle.json must exist.
//
// Materialize runs this before it touches anything, so a fixture whose committed
// digests have drifted from its committed bytes fails with a message naming the
// manifest rather than surfacing several stages later as a mismatched layer.
func VerifyLayerDigests(root string) error {
	return eachManifest(filepath.Join(root, "layers"), func(manifestPath string, data []byte) error {
		var m spec.LayerManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("testregistry: parsing %q: %w", manifestPath, err)
		}
		dir := filepath.Dir(manifestPath)
		sqfs := filepath.Join(dir, "layer.sqfs")

		blob, err := os.ReadFile(sqfs)
		if err != nil {
			return fmt.Errorf("testregistry: reading %q: %w", sqfs, err)
		}
		sum := sha256.Sum256(blob)
		if got := hex.EncodeToString(sum[:]); got != m.SHA256 {
			return fmt.Errorf("testregistry: fixture drift: %s declares sha256 %q but %s hashes to %q",
				manifestPath, m.SHA256, sqfs, got)
		}
		if int64(len(blob)) != m.Size {
			return fmt.Errorf("testregistry: fixture drift: %s declares size %d but %s is %d bytes",
				manifestPath, m.Size, sqfs, len(blob))
		}
		if _, err := os.Stat(filepath.Join(dir, "bundle.json")); err != nil {
			return fmt.Errorf("testregistry: %s: %w", manifestPath, err)
		}
		return nil
	})
}

// rewriteLayerManifests fills in source and bundle for every layer manifest
// under root. They are committed empty because they are absolute file:// URIs.
func rewriteLayerManifests(root string) error {
	return eachManifest(filepath.Join(root, "layers"), func(manifestPath string, data []byte) error {
		var m spec.LayerManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("testregistry: parsing %q: %w", manifestPath, err)
		}
		dir := filepath.Dir(manifestPath)
		m.Source = uriFor(filepath.Join(dir, "layer.sqfs"))
		m.Bundle = uriFor(filepath.Join(dir, "bundle.json"))
		return writeYAML(manifestPath, &m)
	})
}

// rewriteFormationManifests fills in bundle for every formation manifest under
// root.
func rewriteFormationManifests(root string) error {
	return eachManifest(filepath.Join(root, "formations"), func(manifestPath string, data []byte) error {
		var f spec.Formation
		if err := yaml.Unmarshal(data, &f); err != nil {
			return fmt.Errorf("testregistry: parsing %q: %w", manifestPath, err)
		}
		bundle := filepath.Join(filepath.Dir(manifestPath), "bundle.json")
		if _, err := os.Stat(bundle); err != nil {
			return fmt.Errorf("testregistry: %s: %w", manifestPath, err)
		}
		f.Bundle = uriFor(bundle)
		return writeYAML(manifestPath, &f)
	})
}

// eachManifest calls fn for every manifest.yaml under dir. A missing dir is not
// an error: a fixture is allowed to carry only layers, or only formations.
func eachManifest(dir string, fn func(path string, data []byte) error) error {
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "manifest.yaml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("testregistry: reading %q: %w", p, err)
		}
		return fn(p, data)
	})
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// writeYAML marshals v to path.
func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("testregistry: marshalling %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("testregistry: writing %q: %w", path, err)
	}
	return nil
}

// uriFor returns the file:// URI for an absolute filesystem path, using the
// same construction as registry.LocalClient.
func uriFor(absPath string) string {
	return "file://" + filepath.ToSlash(absPath)
}

// ProfileBytes returns the contents of a fixture profile.
// name is one of the Profile* constants.
func ProfileBytes(name string) ([]byte, error) {
	data, err := fixture.ReadFile(path.Join(profilesRoot, name))
	if err != nil {
		return nil, fmt.Errorf("testregistry: no fixture profile %q: %w", name, err)
	}
	return data, nil
}

// ProfileNames lists every fixture profile.
func ProfileNames() []string {
	return []string{ProfileMinimal, ProfileFormation}
}
