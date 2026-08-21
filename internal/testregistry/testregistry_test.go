package testregistry_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// TestMaterializeProducesResolvableRegistry checks the three properties the
// resolver depends on, which are exactly the three the embedded recipe catalog
// lacks: a layer index, a real sha256, and a bundle that exists.
func TestMaterializeProducesResolvableRegistry(t *testing.T) {
	root, client := testregistry.New(t)

	layers, err := client.ListLayers(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("ListLayers: %v", err)
	}
	if len(layers) != len(testregistry.LayerIDs) {
		t.Fatalf("index lists %d layers, want %d", len(layers), len(testregistry.LayerIDs))
	}

	for _, m := range layers {
		if m.SHA256 == "" {
			t.Errorf("%s: empty sha256", m.ID)
		}
		if m.RekorEntry == "" {
			t.Errorf("%s: empty rekor_entry", m.ID)
		}
		bundlePath, ok := stripFileURI(m.Bundle)
		if !ok {
			t.Errorf("%s: bundle %q is not a file:// URI", m.ID, m.Bundle)
			continue
		}
		if _, err := os.Stat(bundlePath); err != nil {
			t.Errorf("%s: bundle does not exist: %v", m.ID, err)
		}
		srcPath, ok := stripFileURI(m.Source)
		if !ok {
			t.Errorf("%s: source %q is not a file:// URI", m.ID, m.Source)
			continue
		}
		if _, err := os.Stat(srcPath); err != nil {
			t.Errorf("%s: layer.sqfs does not exist: %v", m.ID, err)
		}
	}

	if !filepath.IsAbs(root) {
		t.Errorf("Materialize returned relative root %q", root)
	}
}

// TestMaterializeFormation checks the formation reaches the same standard as the
// layers: stage 2 rejects a formation with an empty bundle or rekor_entry.
func TestMaterializeFormation(t *testing.T) {
	_, client := testregistry.New(t)

	f, err := client.ResolveFormation(context.Background(), testregistry.FormationRef, testregistry.BaseArch)
	if err != nil {
		t.Fatalf("ResolveFormation(%s): %v", testregistry.FormationRef, err)
	}
	if f.Bundle == "" || f.RekorEntry == "" {
		t.Errorf("formation has empty bundle (%q) or rekor_entry (%q)", f.Bundle, f.RekorEntry)
	}
	if len(f.Layers) != len(testregistry.LayerIDs) {
		t.Errorf("formation lists %d layers, want %d", len(f.Layers), len(testregistry.LayerIDs))
	}
}

// TestMaterializeDetectsFixtureDrift is the check that keeps the fixture
// honest: if a committed sha256 stops describing the committed bytes, the
// failure must name the file rather than surface later as a mysterious
// verification error.
func TestMaterializeDetectsFixtureDrift(t *testing.T) {
	root, _ := testregistry.New(t)

	sqfs := filepath.Join(root, "layers", "linux-gnu-2.34", "x86_64", "python", "3.13.2", "layer.sqfs")
	data, err := os.ReadFile(sqfs)
	if err != nil {
		t.Fatalf("reading fixture blob: %v", err)
	}
	if err := os.WriteFile(sqfs, append(data, '!'), 0o644); err != nil {
		t.Fatalf("mutating fixture blob: %v", err)
	}

	err = testregistry.VerifyLayerDigests(root)
	if err == nil {
		t.Fatal("VerifyLayerDigests accepted a blob whose bytes do not match the manifest sha256")
	}
	if !strings.Contains(err.Error(), "manifest.yaml") {
		t.Errorf("drift error does not name the manifest: %v", err)
	}
}

// TestFixtureProfilesParse keeps the fixture profiles from drifting away from
// the spec, and pins them to the base OS whose ABI the fixture layers carry:
// they are the input to every offline resolution test.
func TestFixtureProfilesParse(t *testing.T) {
	for _, name := range testregistry.ProfileNames() {
		t.Run(name, func(t *testing.T) {
			data, err := testregistry.ProfileBytes(name)
			if err != nil {
				t.Fatalf("%v", err)
			}
			p, err := spec.ParseProfileBytes(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if p.Base.OS != testregistry.Base {
				t.Errorf("base.os = %q, want %q — fixture layers exist only for that ABI", p.Base.OS, testregistry.Base)
			}
			if p.Base.NormalizedArch() != testregistry.BaseArch {
				t.Errorf("base.arch = %q, want %q", p.Base.NormalizedArch(), testregistry.BaseArch)
			}
		})
	}
}

// realRekorLogKeyID is the key ID of the public-good Sigstore Rekor log, base64
// of c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d. It is
// here only so the fixture can assert it does *not* carry it. An earlier version
// of this fixture did.
const realRekorLogKeyID = "wNI9atQGlz+VWfO6LRygH4QUfY/8W4RFwiT5i5WRgB0="

// TestFixtureIsNotTrustworthy asserts the fixture cannot pass a real
// verification, and is the guard that makes that property survive people.
//
// It enumerates the tree rather than checking the files that exist today: a test
// naming three known values does not constrain the fourth layer someone adds
// next month with another plausible-looking Rekor index. Every manifest and
// every bundle found under the materialized root must carry the Sentinel*
// values.
//
// If a correct tightening of verification (#57, #59, #60, #61) makes this test
// fail, the fixture is what needs new material — not the check.
func TestFixtureIsNotTrustworthy(t *testing.T) {
	// The constants themselves first. Enumeration below asserts equality against
	// them, so a plausible value substituted *into* a constant would leave every
	// other assertion here passing. These pin the properties that make them safe.
	t.Run("sentinels cannot name real material", func(t *testing.T) {
		idx, err := strconv.ParseInt(testregistry.SentinelRekorEntry, 10, 64)
		if err != nil {
			t.Fatalf("SentinelRekorEntry %q must parse as int64 so it reaches the Rekor client and fails there rather than earlier: %v",
				testregistry.SentinelRekorEntry, err)
		}
		// The public log is O(10^8) entries. Anything below this bound could name
		// a real entry belonging to someone else.
		const impossible = int64(1) << 50
		if idx < impossible {
			t.Errorf("SentinelRekorEntry = %d, small enough to be a real log index; want >= %d", idx, impossible)
		}
		if testregistry.SentinelLogKeyID == realRekorLogKeyID {
			t.Error("SentinelLogKeyID is the real public-good Rekor log key ID — the fixture must not claim inclusion in that log")
		}
		sig, err := base64.StdEncoding.DecodeString(testregistry.SentinelSignature)
		if err != nil {
			t.Fatalf("SentinelSignature is not base64: %v", err)
		}
		if !isPrintableASCII(sig) {
			t.Error("SentinelSignature must decode to printable ASCII — that is what makes it impossible to mistake for a signature")
		}
	})

	root, _ := testregistry.New(t)

	manifests, bundles := 0, 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch d.Name() {
		case "manifest.yaml":
			manifests++
			checkManifestRekor(t, p)
		case "bundle.json":
			bundles++
			checkBundleIsFake(t, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking fixture: %v", err)
	}

	// A walk that found nothing would pass every assertion above vacuously,
	// which is the same fail-open shape this test exists to prevent.
	if manifests == 0 || bundles == 0 {
		t.Fatalf("enumerated %d manifests and %d bundles under %s; both must be non-zero or this test proves nothing",
			manifests, bundles, root)
	}
	t.Logf("checked %d manifests and %d bundles", manifests, bundles)
}

// checkManifestRekor asserts one manifest carries the sentinel Rekor entry. It
// covers layer and formation manifests alike: both are read by a stage that will
// eventually verify them.
func checkManifestRekor(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("reading %s: %v", path, err)
		return
	}
	var m struct {
		RekorEntry string `yaml:"rekor_entry"`
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Errorf("parsing %s: %v", path, err)
		return
	}
	if m.RekorEntry != testregistry.SentinelRekorEntry {
		t.Errorf("%s: rekor_entry = %q, want SentinelRekorEntry %q — a fixture must not carry an index that could name a real log entry",
			path, m.RekorEntry, testregistry.SentinelRekorEntry)
	}
}

// checkBundleIsFake asserts one bundle.json carries only sentinel trust material
// and says so about itself.
func checkBundleIsFake(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("reading %s: %v", path, err)
		return
	}
	var b struct {
		Note                 string `json:"_strata_fixture"`
		VerificationMaterial struct {
			TlogEntries []struct {
				LogIndex string `json:"logIndex"`
				LogID    struct {
					KeyID string `json:"keyId"`
				} `json:"logId"`
				IntegratedTime string `json:"integratedTime"`
			} `json:"tlogEntries"`
		} `json:"verificationMaterial"`
		MessageSignature struct {
			Signature string `json:"signature"`
		} `json:"messageSignature"`
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Errorf("parsing %s: %v", path, err)
		return
	}

	if b.Note == "" {
		t.Errorf("%s: no _strata_fixture note — a fake bundle must identify itself in its own bytes", path)
	}
	if b.MessageSignature.Signature != testregistry.SentinelSignature {
		t.Errorf("%s: signature = %q, want SentinelSignature", path, b.MessageSignature.Signature)
	}
	if len(b.VerificationMaterial.TlogEntries) == 0 {
		t.Errorf("%s: no tlogEntries to check", path)
		return
	}
	for i, e := range b.VerificationMaterial.TlogEntries {
		if e.LogIndex != testregistry.SentinelRekorEntry {
			t.Errorf("%s: tlogEntries[%d].logIndex = %q, want SentinelRekorEntry %q",
				path, i, e.LogIndex, testregistry.SentinelRekorEntry)
		}
		if e.LogID.KeyID != testregistry.SentinelLogKeyID {
			t.Errorf("%s: tlogEntries[%d].logId.keyId = %q, want SentinelLogKeyID — the fixture must not claim a real log's key",
				path, i, e.LogID.KeyID)
		}
		if e.IntegratedTime != testregistry.SentinelIntegratedTime {
			t.Errorf("%s: tlogEntries[%d].integratedTime = %q, want SentinelIntegratedTime %q",
				path, i, e.IntegratedTime, testregistry.SentinelIntegratedTime)
		}
	}
}

// isPrintableASCII reports whether b is non-empty printable ASCII.
func isPrintableASCII(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return len(b) > 0
}

func stripFileURI(u string) (string, bool) {
	const prefix = "file://"
	if len(u) <= len(prefix) || u[:len(prefix)] != prefix {
		return "", false
	}
	return u[len(prefix):], true
}
