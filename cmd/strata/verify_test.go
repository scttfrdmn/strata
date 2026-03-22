package main

import (
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func TestCollectPresenceFailures_AllPresent(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "12345",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:         "python-3.11.11",
					Bundle:     "bundle-json-data",
					RekorEntry: "111",
				},
			},
			{
				LayerManifest: spec.LayerManifest{
					ID:         "gcc-13.2.0",
					Bundle:     "bundle-json-data",
					RekorEntry: "222",
				},
			},
		},
	}
	failures := collectPresenceFailures(lf)
	if len(failures) != 0 {
		t.Errorf("expected no failures, got: %v", failures)
	}
}

func TestCollectPresenceFailures_UnsignedLockfile(t *testing.T) {
	lf := &spec.LockFile{
		// RekorEntry deliberately empty
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:         "python-3.11.11",
					Bundle:     "bundle",
					RekorEntry: "111",
				},
			},
		},
	}
	failures := collectPresenceFailures(lf)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure (unsigned lockfile), got %d: %v", len(failures), failures)
	}
	if !containsStr(failures[0], "RekorEntry") {
		t.Errorf("failure message should mention RekorEntry: %q", failures[0])
	}
}

func TestCollectPresenceFailures_MissingBundle(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "99",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:         "python-3.11.11",
					Bundle:     "", // missing
					RekorEntry: "111",
				},
			},
		},
	}
	failures := collectPresenceFailures(lf)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure (missing bundle), got %d: %v", len(failures), failures)
	}
	if !containsStr(failures[0], "Bundle") {
		t.Errorf("failure message should mention Bundle: %q", failures[0])
	}
}

func TestCollectPresenceFailures_MissingRekorEntry(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "99",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:         "gcc-13.2.0",
					Bundle:     "bundle",
					RekorEntry: "", // missing
				},
			},
		},
	}
	failures := collectPresenceFailures(lf)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
	if !containsStr(failures[0], "RekorEntry") {
		t.Errorf("failure message should mention RekorEntry: %q", failures[0])
	}
}

func TestCollectPresenceFailures_MultipleFailures(t *testing.T) {
	lf := &spec.LockFile{
		// Unsigned lockfile + two unsigned layers = 5 failures
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "a"}},
			{LayerManifest: spec.LayerManifest{ID: "b"}},
		},
	}
	failures := collectPresenceFailures(lf)
	// 1 lockfile + 2*(bundle+rekor) = 5
	if len(failures) != 5 {
		t.Errorf("expected 5 failures, got %d: %v", len(failures), failures)
	}
}

func TestCollectPresenceFailures_EmptyLayers(t *testing.T) {
	lf := &spec.LockFile{RekorEntry: "1"}
	failures := collectPresenceFailures(lf)
	if len(failures) != 0 {
		t.Errorf("expected no failures for signed lockfile with no layers, got: %v", failures)
	}
}

// containsStr is a helper to check if s contains substr (reuse existing definition
// from fold_test.go would conflict due to different packages, so define locally).
func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
