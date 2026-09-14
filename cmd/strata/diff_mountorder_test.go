package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestDiff_ReportsMountOrderReorder is #154: two lockfiles that differ only in
// mount order must not report "No differences found." The lockfiles are unfrozen
// (no SHA256, so EnvironmentID is "" for both) precisely so the top-level id diff
// cannot fire — the layer-level comparison is the only thing that can catch the
// reorder, and before the fix it ignored MountOrder and called the layers
// unchanged. Mount order is the OverlayFS lower-stack position, so a reorder is a
// real difference.
func TestDiff_ReportsMountOrderReorder(t *testing.T) {
	mk := func(o1, o2 int) *spec.LockFile {
		return &spec.LockFile{
			Layers: []spec.ResolvedLayer{
				{LayerManifest: spec.LayerManifest{Name: "alpha", Version: "1.0"}, MountOrder: o1},
				{LayerManifest: spec.LayerManifest{Name: "bravo", Version: "1.0"}, MountOrder: o2},
			},
		}
	}
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.lock.yaml")
	p2 := filepath.Join(dir, "b.lock.yaml")
	if err := writeYAML(p1, mk(1, 2)); err != nil {
		t.Fatalf("write %s: %v", p1, err)
	}
	if err := writeYAML(p2, mk(2, 1)); err != nil {
		t.Fatalf("write %s: %v", p2, err)
	}

	out := captureStdout(t, func() error { return runDiff(p1, p2) })

	if strings.Contains(out.text, "No differences found") {
		t.Errorf("diff reported no difference for a pure mount-order reorder (#154):\n%s", out.text)
	}
	if out.err == nil {
		t.Error("runDiff returned nil (exit 0) for a mount-order reorder; a diagnostic that exits 0 on a real change is worse than none")
	}
	if !strings.Contains(out.text, "mount order") {
		t.Errorf("diff did not name the mount-order change:\n%s", out.text)
	}
}

type diffCapture struct {
	text string
	err  error
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what it
// printed alongside its error.
func captureStdout(t *testing.T, fn func() error) diffCapture {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	return diffCapture{text: string(data), err: runErr}
}
