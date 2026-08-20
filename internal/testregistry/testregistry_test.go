package testregistry_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func stripFileURI(u string) (string, bool) {
	const prefix = "file://"
	if len(u) <= len(prefix) || u[:len(prefix)] != prefix {
		return "", false
	}
	return u[len(prefix):], true
}
