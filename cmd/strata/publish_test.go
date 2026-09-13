package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// A 64-hex string so the fixtures are frozen under IsFrozen today. (That
// IsFrozen accepts any non-empty string rather than a real digest is #96 — not
// this test's subject; these gates fire on top of whatever IsFrozen decides.)
const publishTestDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// writePublishFixture marshals lf to a temp file and returns the path.
func writePublishFixture(t *testing.T, lf spec.LockFile) string {
	t.Helper()
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatalf("marshal lockfile: %v", err)
	}
	p := filepath.Join(t.TempDir(), "lock.yaml")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}
	return p
}

// publishableLockFile is frozen, clean and signed — the only shape publish may
// accept. Each rejection test below flips exactly one of those properties off,
// so a refusal is attributable to that single field rather than to some other
// missing one; the accepts-test is the non-vacuity anchor that proves the base
// fixture is otherwise valid.
func publishableLockFile() spec.LockFile {
	return spec.LockFile{
		Base:       spec.ResolvedBase{AMISHA256: publishTestDigest},
		RekorEntry: "rekor-entry-1",
	}
}

func TestParsePublishableLockFile_AcceptsFrozenCleanSigned(t *testing.T) {
	if _, err := parsePublishableLockFile(writePublishFixture(t, publishableLockFile())); err != nil {
		t.Fatalf("a frozen, clean, signed lockfile must be publishable, got: %v", err)
	}
}

func TestParsePublishableLockFile_RejectsUnfrozen(t *testing.T) {
	lf := publishableLockFile()
	lf.Base.AMISHA256 = ""
	assertPublishRejected(t, lf, "not frozen")
}

func TestParsePublishableLockFile_RejectsDirtyEnvironment(t *testing.T) {
	lf := publishableLockFile()
	lf.MutableLayer = &spec.MutableLayerSpec{Name: "dirty-upper", Version: "0.1.0"}
	assertPublishRejected(t, lf, "mutable upper layer")
}

func TestParsePublishableLockFile_RejectsUnsigned(t *testing.T) {
	lf := publishableLockFile()
	lf.RekorEntry = ""
	assertPublishRejected(t, lf, "not signed")
}

func assertPublishRejected(t *testing.T, lf spec.LockFile, wantReason string) {
	t.Helper()
	_, err := parsePublishableLockFile(writePublishFixture(t, lf))
	if err == nil {
		t.Fatalf("expected publish to refuse the lockfile, got nil error")
	}
	if !strings.Contains(err.Error(), wantReason) {
		t.Fatalf("refusal must name the reason %q, got: %v", wantReason, err)
	}
}
