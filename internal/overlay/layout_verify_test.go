package overlay_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/internal/overlay"
)

// makeVersionedLayer returns a temp layer root containing <name>/<version>/bin,
// the structure a real versioned layer ships and the one PATH is built from.
func makeVersionedLayer(t *testing.T, name, version string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, name, version, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestVerifyLayerLayout covers #146: the manifest triple is unsigned, so the
// check derives the layout from the mounted bytes. The two passing cases are the
// non-vacuity anchors; the refusals are the two attacks the issue names (version
// bump, flat relabel) plus the malformed-field guards.
func TestVerifyLayerLayout(t *testing.T) {
	t.Run("versioned layer with matching bin passes", func(t *testing.T) {
		root := makeVersionedLayer(t, "python", "3.11.9")
		if err := overlay.VerifyLayerLayout(root, "python", "3.11.9", ""); err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})
	t.Run("genuine flat layer passes", func(t *testing.T) {
		root := t.TempDir() // no <name>/<version>/bin
		if err := overlay.VerifyLayerLayout(root, "somebin", "1.0.0", "flat"); err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})

	t.Run("versioned layer missing bin is refused", func(t *testing.T) {
		if err := overlay.VerifyLayerLayout(t.TempDir(), "python", "3.11.9", ""); err == nil {
			t.Fatal("want refusal for a versioned layer with no <name>/<version>/bin")
		}
	})
	t.Run("version-bump attack is refused", func(t *testing.T) {
		root := makeVersionedLayer(t, "python", "3.11.9") // bytes provide 3.11.9
		if err := overlay.VerifyLayerLayout(root, "python", "3.11.10", ""); err == nil {
			t.Fatal("want refusal: manifest claims 3.11.10 but the layer provides 3.11.9")
		}
	})
	t.Run("flat-relabel attack is refused", func(t *testing.T) {
		root := makeVersionedLayer(t, "python", "3.11.9") // really versioned
		if err := overlay.VerifyLayerLayout(root, "python", "3.11.9", "flat"); err == nil {
			t.Fatal("want refusal: a versioned layer relabeled install_layout=flat")
		}
	})
	t.Run("name with traversal is refused", func(t *testing.T) {
		if err := overlay.VerifyLayerLayout(t.TempDir(), "../etc", "1.0.0", ""); err == nil {
			t.Fatal("want refusal for a name that is not a safe path component")
		}
	})
	t.Run("empty version is refused", func(t *testing.T) {
		if err := overlay.VerifyLayerLayout(t.TempDir(), "python", "", ""); err == nil {
			t.Fatal("want refusal for an empty version")
		}
	})
}
