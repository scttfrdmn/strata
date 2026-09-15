package spec

import (
	"strings"
	"testing"
)

// TestVerificationPolicy_InertRouteHasNoReader guards the inert route
// LockFile.VerificationPolicy takes (#100). It is recorded for audit and read by
// a consumer inspecting the artifact, but no in-tree assembler reads it, which is
// why two lockfiles differing only in this label share an EnvironmentID.
//
// The generic route test (TestEnvironmentIDRoutes_MatchBehaviour) only checks
// that mutating the field does not move the identity — it cannot see a future
// assembler that branches on lf.VerificationPolicy without adding it to
// envHashInput, which would make the field influence behaviour while staying out
// of the identity. This scans every non-test .go file for the identifier and
// requires each mention outside the declare and write sites to be classified,
// the same discipline the declared-provenance no-reader test applies to exported
// env names.
func TestVerificationPolicy_InertRouteHasNoReader(t *testing.T) {
	root := moduleRoot(t)
	goFiles := collectFiles(t, root, isGoNonTest)
	if len(goFiles) == 0 {
		t.Fatalf("scanned 0 non-test .go files under %s; a zero-mentions result would mean nothing", root)
	}

	// The only sites that may name the identifier: spec/lockfile.go declares the
	// field and the Verify* constants; internal/resolver/stages.go writes it.
	// Neither reads lf.VerificationPolicy to drive assembly.
	allowed := map[string]bool{
		"spec/lockfile.go":            true,
		"internal/resolver/stages.go": true,
	}

	seen := 0
	for rel, content := range goFiles {
		if !strings.Contains(content, "VerificationPolicy") {
			continue
		}
		seen++
		if allowed[rel] {
			continue
		}
		t.Errorf("%s names VerificationPolicy outside the declare/write sites. If it reads "+
			"lf.VerificationPolicy to drive assembly, the field's `inert` route "+
			"(spec/environment_id_scope_test.go) no longer holds and it must move into "+
			"envHashInput (#100); if it is another write or a doc mention, add it to the "+
			"allowlist here with its purpose.", rel)
	}

	// Non-vacuity: the two known sites must be seen, or the scan read an empty
	// domain and its silence means nothing.
	if seen < 2 {
		t.Fatalf("expected VerificationPolicy at its declare and write sites, saw %d — the scan is broken", seen)
	}
}
