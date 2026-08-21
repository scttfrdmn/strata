package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// This is the regression test for #57: the strata run layer cache was an
// integrity bypass built from three defects that compose. The path was built
// from an unvalidated lockfile field, a cache hit was never re-hashed, and an
// empty digest both disabled verification and collided every hashless layer
// onto the single filename ".sqfs".
//
// The unit under test is fetchLayersToCache, not the strata binary, because
// fetchLayersToCache is what all four consumers share — run.go:102,
// export.go:76, fold.go:122 and fold.go:172 — and every path it returns goes
// straight to overlay assembly. Testing it here covers all four.
//
// Every rejection case below is paired with a control. A test that only asserts
// "this is rejected" passes just as well when everything is rejected, and a
// tightening that rejects everything is not a fix. The controls are what make
// these measurements.

// fixtureLockfile returns a lockfile whose layers are the fixture registry's
// real layers: real digests over the real committed bytes, file:// sources that
// exist. It is the honest input the rejection cases are measured against.
func fixtureLockfile(t *testing.T) spec.LockFile {
	t.Helper()
	_, client := testregistry.New(t)
	manifests, err := client.ListLayers(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("listing fixture layers: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("fixture registry has no layers — every assertion below would pass vacuously")
	}
	lf := spec.LockFile{}
	for i, m := range manifests {
		if m.SHA256 == "" || m.Source == "" {
			t.Fatalf("fixture layer %q has sha256=%q source=%q", m.ID, m.SHA256, m.Source)
		}
		lf.Layers = append(lf.Layers, spec.ResolvedLayer{
			LayerManifest: *m,
			MountOrder:    i + 1,
		})
	}
	return lf
}

// cacheDir returns a fresh, existing cache directory and the work directory
// containing it. The work directory is what "outside the cache" means below.
func cacheDir(t *testing.T) (work, cache string) {
	t.Helper()
	work = t.TempDir()
	cache = filepath.Join(work, "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	return work, cache
}

// TestLayerCacheAcceptsHonestLayers is the control for every rejection test in
// this file: the fixture's real layers must still fetch, and must still be
// reused on a second call, which is the path that now re-hashes. If this fails,
// the tightening is rejecting valid material and nothing else here means
// anything.
func TestLayerCacheAcceptsHonestLayers(t *testing.T) {
	lf := fixtureLockfile(t)
	_, cache := cacheDir(t)

	first, err := fetchLayersToCache(context.Background(), lf, cache)
	if err != nil {
		t.Fatalf("fetching the fixture's own layers: %v", err)
	}
	if len(first) != len(lf.Layers) {
		t.Fatalf("fetched %d layers, want %d", len(first), len(lf.Layers))
	}
	for _, lp := range first {
		if filepath.Dir(lp.Path) != cache {
			t.Errorf("%s: path %q is outside the cache dir %q", lp.ID, lp.Path, cache)
		}
		if err := spec.VerifyFileDigest(lp.Path, lp.SHA256); err != nil {
			t.Errorf("%s: returned path does not hash to its declared digest: %v", lp.ID, err)
		}
	}

	// Second call takes the cache-hit branch for every layer.
	second, err := fetchLayersToCache(context.Background(), lf, cache)
	if err != nil {
		t.Fatalf("second fetch (cache hits, now re-hashed): %v", err)
	}
	for i := range second {
		if second[i].Path != first[i].Path {
			t.Errorf("%s: cache hit returned %q, first fetch returned %q",
				second[i].ID, second[i].Path, first[i].Path)
		}
	}
}

// TestLayerCacheRejectsTraversalDigest is the issue's composed bypass, with its
// negative control.
//
// A lockfile declaring sha256 "../escaped" named a file outside --cache-dir,
// because filepath.Join calls Clean, which resolves ".." rather than rejecting
// it. On its own that was self-cleaning: the post-download hash check removed
// the file. It was the unhashed cache hit that made it persist — if the escaped
// path already existed, the layer was satisfied by whatever was there and
// nothing was ever hashed.
//
// The control is the same lockfile with the planted file removed. Before the
// fix the two arms behaved differently: with the file present the run reached
// mount, and without it the run died fetching a source that never existed. That
// difference is precisely the measurement that the planted content was what
// satisfied the layer. After the fix the two arms must be indistinguishable —
// the digest is rejected before anything on disk is consulted.
func TestLayerCacheRejectsTraversalDigest(t *testing.T) {
	errText := map[bool]string{}

	for _, planted := range []bool{true, false} {
		work, cache := cacheDir(t)

		// The escaped path is cache/../escaped.sqfs.
		escaped := filepath.Join(work, "escaped.sqfs")
		if planted {
			if err := os.WriteFile(escaped, []byte("planted content, hashes to nothing declared"), 0o600); err != nil {
				t.Fatalf("planting: %v", err)
			}
		}

		lf := spec.LockFile{Layers: []spec.ResolvedLayer{{
			LayerManifest: spec.LayerManifest{
				ID:     "victim",
				SHA256: "../escaped",
				// A source that does not exist: if the run gets as far as
				// fetching, it fails, and the failure names the source. That is
				// how the control distinguishes "rejected the digest" from
				// "tried to download and could not".
				Source: "file://" + filepath.Join(work, "nonexistent-source.sqfs"),
			},
			MountOrder: 1,
		}}}

		paths, err := fetchLayersToCache(context.Background(), lf, cache)
		if err == nil {
			t.Fatalf("planted=%v: fetchLayersToCache accepted a traversal digest and returned %+v", planted, paths)
		}
		if paths != nil {
			t.Errorf("planted=%v: paths returned alongside an error: %+v", planted, paths)
		}
		if !strings.Contains(err.Error(), "layer digest") {
			t.Errorf("planted=%v: want a digest rejection, got: %v", planted, err)
		}
		if strings.Contains(err.Error(), "nonexistent-source") {
			t.Errorf("planted=%v: the digest reached the fetch stage before being rejected: %v", planted, err)
		}
		if planted {
			if _, statErr := os.Stat(escaped); statErr != nil {
				t.Errorf("the planted file outside the cache dir was touched: %v", statErr)
			}
		}
		errText[planted] = err.Error()
	}

	if errText[true] != errText[false] {
		t.Errorf("the outcome still depends on the planted file:\n  present: %s\n  absent:  %s",
			errText[true], errText[false])
	}
}

// TestLayerCacheRejectsPlantedCacheHit covers defect 2 on its own: a file at a
// perfectly well-formed <64hex>.sqfs name inside the cache directory, whose
// bytes are not the bytes that digest names. Validating the path does not help
// here — the name is valid. Only hashing the hit does.
//
// The control is the same test with the correct bytes planted, which must
// succeed: it is what proves the rejection above is about the contents and not
// about cache hits in general.
func TestLayerCacheRejectsPlantedCacheHit(t *testing.T) {
	lf := fixtureLockfile(t)
	lf.Layers = lf.Layers[:1]
	layer := lf.Layers[0]

	honest, err := os.ReadFile(strings.TrimPrefix(layer.Source, "file://"))
	if err != nil {
		t.Fatalf("reading the fixture layer's real bytes: %v", err)
	}

	t.Run("planted content is rejected", func(t *testing.T) {
		_, cache := cacheDir(t)
		cached := filepath.Join(cache, layer.SHA256+".sqfs")
		if err := os.WriteFile(cached, []byte("not the layer these bytes are named after"), 0o600); err != nil {
			t.Fatal(err)
		}

		paths, err := fetchLayersToCache(context.Background(), lf, cache)
		if err == nil {
			t.Fatalf("a cache hit whose contents are wrong was accepted: %+v", paths)
		}
		var mismatch *spec.DigestMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("want a *spec.DigestMismatchError, got: %v", err)
		}
		if mismatch.Want != layer.SHA256 {
			t.Errorf("mismatch names declared digest %q, want %q", mismatch.Want, layer.SHA256)
		}
		// The file is left in place deliberately: it is the operator's
		// evidence, and this is a read path. The error says to remove it.
		if _, statErr := os.Stat(cached); statErr != nil {
			t.Errorf("the rejected cache file was deleted: %v", statErr)
		}
		if !strings.Contains(err.Error(), "remove the file") {
			t.Errorf("the error does not say how to recover: %v", err)
		}
	})

	t.Run("honest content is accepted", func(t *testing.T) {
		_, cache := cacheDir(t)
		cached := filepath.Join(cache, layer.SHA256+".sqfs")
		if err := os.WriteFile(cached, honest, 0o600); err != nil {
			t.Fatal(err)
		}

		paths, err := fetchLayersToCache(context.Background(), lf, cache)
		if err != nil {
			t.Fatalf("a cache hit with the right contents was rejected: %v", err)
		}
		if len(paths) != 1 || paths[0].Path != cached {
			t.Fatalf("got %+v, want the cached path %q", paths, cached)
		}
	})
}

// TestLayerCacheRejectsEmptyDigest covers defect 3, both halves: an empty digest
// disabled verification entirely (the post-download check was guarded by
// `if layer.SHA256 != ""`), and it collided, because Join(cacheDir, ""+".sqfs")
// is ".sqfs" for every hashless layer in every lockfile — so the first one
// downloaded satisfied all the others through the cache-hit path.
//
// The control is the same two layers with their real digests, which must fetch
// to two distinct files. Otherwise "no collision" could be true only because
// nothing was fetched at all.
func TestLayerCacheRejectsEmptyDigest(t *testing.T) {
	fixture := fixtureLockfile(t)
	if len(fixture.Layers) < 2 {
		t.Fatalf("need two fixture layers to test the collision, have %d", len(fixture.Layers))
	}
	a, b := fixture.Layers[0], fixture.Layers[1]

	t.Run("rejected", func(t *testing.T) {
		_, cache := cacheDir(t)
		hashless := spec.LockFile{Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: a.ID, Source: a.Source}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{ID: b.ID, Source: b.Source}, MountOrder: 2},
		}}

		paths, err := fetchLayersToCache(context.Background(), hashless, cache)
		if err == nil {
			t.Fatalf("layers with no digest were accepted: %+v", paths)
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("want an empty-digest rejection, got: %v", err)
		}

		entries, readErr := os.ReadDir(cache)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Errorf("a rejected fetch wrote %d file(s) into the cache: %v", len(entries), entries)
		}
	})

	t.Run("control: the same two layers with real digests do not collide", func(t *testing.T) {
		_, cache := cacheDir(t)
		honest := spec.LockFile{Layers: []spec.ResolvedLayer{a, b}}

		paths, err := fetchLayersToCache(context.Background(), honest, cache)
		if err != nil {
			t.Fatalf("two honest layers: %v", err)
		}
		if len(paths) != 2 {
			t.Fatalf("got %d paths, want 2", len(paths))
		}
		if paths[0].Path == paths[1].Path {
			t.Errorf("two layers share one cache file: %q", paths[0].Path)
		}
	})
}
