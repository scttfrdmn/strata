// Package testregistry materializes a valid file:// Strata registry from
// repo-resident fixture data, so that resolution can reach stage 8 with no AWS
// credentials, no network, and no prior build.
//
// The embedded Tier 0 catalog is *recipes*: it carries no sha256, bundle, or
// rekor_entry, because nothing has been built. Resolution against it therefore
// dies in stage 7 (BUNDLE_MISSING) for every profile. This package is the
// offline substitute — a two-layer registry whose manifests carry a real
// sha256 over real bytes, a bundle that exists on disk, and a Rekor entry that
// parses as an integer but cannot name a real log entry.
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
// # Why the fake fields are shaped the way they are
//
// Fixture data must be *maximally implausible to a real verifier*, not minimally
// sufficient for the checks that happen to exist today. A fixture optimised to
// pass the current checks will also pass whatever replaces them, which is the
// failure mode this package nearly shipped with.
//
// The first version chose rekor_entry values that looked like real log indices,
// reasoning only that stage 7 parses them with strconv.ParseInt when a Rekor
// client is configured. All three resolved to HTTP 200 in the live public log —
// real entries belonging to other people's artifacts — and the bundles carried
// the genuine public-good Rekor log's key ID alongside them. Because
// RekorHTTPClient.VerifyEntry treats existence as proof and discards the bundle
// it was handed (#59), a resolve with a real Rekor client would have reported
// this fixture as transparency-log verified, by borrowing a stranger's
// attestation. Every synthetic field is now chosen so the strongest available
// check fails on it:
//
//   - SentinelRekorEntry parses as int64 and cannot be a log index, so it reaches
//     VerifyEntry and fails there rather than short-circuiting earlier.
//   - SentinelLogKeyID replaces the real log's key ID, which the fixture had no
//     business claiming.
//   - SentinelSignature is ASCII, so it cannot accidentally be a valid signature.
//   - Layer messageDigest values are the real SHA-256 of layer.sqfs, so a digest
//     comparison passes and the *signature* check is what fails. Failing at the
//     right step is more useful than failing early.
//
// TestFixtureIsNotTrustworthy enumerates the whole tree and asserts these hold
// for every file, including files added later, and separately pins the properties
// of the constants themselves so the invariant cannot be defeated by editing one.
// If a correct tightening of verification appears to require loosening a check or
// special-casing this fixture, the fixture is what is wrong.
//
// Extending it is adding files under testdata/registry — Materialize is
// data-driven and discovers whatever manifests are there. It verifies each
// committed sha256 against the committed bytes, so a fixture that drifts fails
// with a message naming the file rather than rotting silently. New layers must
// use the Sentinel* values below; TestFixtureIsNotTrustworthy fails if they do
// not.
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

	// SentinelRekorEntry is the rekor_entry and bundle logIndex carried by every
	// fixture manifest. It is 2^53-1: a valid int64, so it survives
	// strconv.ParseInt in stage 7 and reaches the Rekor client, and far beyond any
	// real log size, so the client cannot find it. Confirmed HTTP 404 against
	// rekor.sigstore.dev on 2026-08-20, where 148923470-148923472 — the plausible
	// indices this fixture used to carry — all returned HTTP 200 for other
	// people's entries.
	//
	// Do not replace this with a realistic-looking index. A fixture that can be
	// confirmed present in a real transparency log is a fixture that can launder a
	// stranger's attestation into a passing test.
	SentinelRekorEntry = "9007199254740991"

	// SentinelLogKeyID is the logId.keyId in every fixture bundle: base64 of the
	// ASCII "strata-fixture-NOT-a-real-log-ID". It replaces
	// c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d, the real
	// public-good Rekor log key ID, which this fixture previously claimed.
	SentinelLogKeyID = "c3RyYXRhLWZpeHR1cmUtTk9ULWEtcmVhbC1sb2ctSUQ="

	// SentinelSignature is the messageSignature.signature in every fixture bundle:
	// base64 of the ASCII "strata-fixture-not-a-real-signature". ASCII text cannot
	// be a valid ECDSA signature, so real verification fails on it immediately and
	// legibly.
	SentinelSignature = "c3RyYXRhLWZpeHR1cmUtbm90LWEtcmVhbC1zaWduYXR1cmU="

	// SentinelIntegratedTime is the tlog integratedTime in every fixture bundle.
	// Zero rather than a plausible recent timestamp: anything that renders it
	// shows 1970 and is obviously looking at fixture data.
	SentinelIntegratedTime = "0"
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
