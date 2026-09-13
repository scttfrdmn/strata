package trust_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// TestVerifyLayers_RejectsEscapingLayerID proves the verify path (#58) refuses a
// layer whose ID would escape squashfsDir when joined — before it touches the
// filesystem or the verifier.
//
// The assertion is on the *message*, not merely that an error occurred: with the
// validator removed, VerifyLayers would still error, but for the wrong reason —
// filepath.Join(dir, "../escape.sqfs") names a nonexistent file and VerifyLayer
// fails to open it. Only asserting "unsafe layer id" distinguishes the guard
// firing from an incidental file-not-found, so the test cannot pass vacuously.
func TestVerifyLayers_RejectsEscapingLayerID(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	lockfile := &spec.LockFile{
		Layers: []spec.ResolvedLayer{{
			LayerManifest: spec.LayerManifest{
				ID:     "../escape",
				SHA256: strings.Repeat("a", 64),
			},
			MountOrder: 1,
		}},
	}

	err := trust.VerifyLayers(ctx, lockfile, dir, &trust.FakeVerifier{})
	if err == nil {
		t.Fatal("VerifyLayers accepted a layer id that escapes squashfsDir; expected refusal")
	}
	if !strings.Contains(err.Error(), "unsafe layer id") {
		t.Fatalf("refusal must name the unsafe id (not an incidental file error), got: %v", err)
	}
}
