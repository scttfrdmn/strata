package fold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// makeSingleLayerLockfile builds a minimal LockFile with one layer for testing.
func makeSingleLayerLockfile(name, version, sha256 string) *spec.LockFile {
	return &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    name,
					Version: version,
					SHA256:  sha256,
				},
				MountOrder: 1,
			},
		},
	}
}

func TestCopyRegularFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "subdir", "dst.txt")

	if err := os.WriteFile(src, []byte("hello strata"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyRegularFile(src, dst, 0644); err != nil {
		t.Fatalf("copyRegularFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != "hello strata" {
		t.Errorf("content = %q, want %q", got, "hello strata")
	}
}

func TestCopyRegularFile_PreservesMode(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "exec.sh")
	dst := filepath.Join(tmp, "exec-copy.sh")

	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := copyRegularFile(src, dst, 0755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestCopyTree_BasicStructure(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	// Build a small tree: src/{bin/tool, lib/foo.so, subdir/file.txt}
	for _, d := range []string{
		filepath.Join(src, "bin"),
		filepath.Join(src, "lib"),
		filepath.Join(src, "subdir"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(src, "bin", "tool"):        "binary content",
		filepath.Join(src, "lib", "foo.so"):      "library content",
		filepath.Join(src, "subdir", "file.txt"): "text content",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyTree(context.Background(), src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	// Verify all files exist at destination.
	for srcPath, wantContent := range files {
		rel, _ := filepath.Rel(src, srcPath)
		dstPath := filepath.Join(dst, rel)
		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("missing file %q: %v", rel, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("file %q content = %q, want %q", rel, got, wantContent)
		}
	}
}

func TestCopyTree_Symlinks(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	if err := os.MkdirAll(filepath.Join(src, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a real file and a symlink to it.
	if err := os.WriteFile(filepath.Join(src, "bin", "python3.11"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("python3.11", filepath.Join(src, "bin", "python3")); err != nil {
		t.Fatal(err)
	}

	if err := copyTree(context.Background(), src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	// Verify symlink exists at destination.
	target, err := os.Readlink(filepath.Join(dst, "bin", "python3"))
	if err != nil {
		t.Fatalf("symlink not preserved: %v", err)
	}
	if target != "python3.11" {
		t.Errorf("symlink target = %q, want python3.11", target)
	}
}

func TestCopyTree_ContextCancellation(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := copyTree(ctx, src, dst)
	if err == nil {
		// Empty dir may complete before context check — that's fine.
		return
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestBuildReadme_ContainsExpectedSections(t *testing.T) {
	lf := makeSingleLayerLockfile("python", "3.11.11", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
	lf.ProfileName = "test-profile"

	readme := buildReadme(lf, "/output/dir")

	for _, want := range []string{
		"Strata Ejected Environment",
		"test-profile",
		"strata fold --eject",
		"/output/dir",
		"python@3.11.11",
	} {
		if !containsSubstr(readme, want) {
			t.Errorf("README missing %q", want)
		}
	}
}

func TestBuildReadme_EmptyProfile(t *testing.T) {
	lf := makeSingleLayerLockfile("gcc", "13.2.0", "aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234")
	readme := buildReadme(lf, "/tmp/out")
	if readme == "" {
		t.Fatal("expected non-empty README")
	}
}

// containsSubstr is a simple string-contains helper.
func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
