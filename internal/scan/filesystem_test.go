package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHasUsableDirs_WithBin(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if !hasUsableDirs(tmp) {
		t.Error("expected hasUsableDirs=true when bin/ exists")
	}
}

func TestHasUsableDirs_WithLib(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	if !hasUsableDirs(tmp) {
		t.Error("expected hasUsableDirs=true when lib/ exists")
	}
}

func TestHasUsableDirs_Empty(t *testing.T) {
	tmp := t.TempDir()
	if hasUsableDirs(tmp) {
		t.Error("expected hasUsableDirs=false for empty dir")
	}
}

func TestHasUsableDirs_OnlyIncludeDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if hasUsableDirs(tmp) {
		t.Error("expected hasUsableDirs=false when only include/ exists (not bin/ or lib/)")
	}
}

func TestDetectFilesystem_DiscoversPkgs(t *testing.T) {
	tmp := t.TempDir()

	// Create <tmp>/python/3.11.11/bin/ and <tmp>/gcc/13.2.0/lib/
	mkpkg := func(name, version, subdir string) {
		d := filepath.Join(tmp, name, version, subdir)
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	mkpkg("python", "3.11.11", "bin")
	mkpkg("gcc", "13.2.0", "lib")
	// No bin or lib — should be skipped.
	if err := os.MkdirAll(filepath.Join(tmp, "headers", "1.0", "include"), 0755); err != nil {
		t.Fatal(err)
	}
	// Non-numeric version — should be skipped.
	mkpkg("tool", "latest", "bin")

	pkgs, err := DetectFilesystem(context.Background(), []string{tmp})
	if err != nil {
		t.Fatal(err)
	}

	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %+v", len(pkgs), pkgs)
	}

	names := make(map[string]string)
	for _, p := range pkgs {
		names[p.Name] = p.Version
		if p.Source != SourceFilesystem {
			t.Errorf("Source = %v, want SourceFilesystem", p.Source)
		}
	}

	if names["python"] != "3.11.11" {
		t.Errorf("python version = %q, want 3.11.11", names["python"])
	}
	if names["gcc"] != "13.2.0" {
		t.Errorf("gcc version = %q, want 13.2.0", names["gcc"])
	}
}

func TestDetectFilesystem_NonexistentPath(t *testing.T) {
	pkgs, err := DetectFilesystem(context.Background(), []string{"/nonexistent/path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestDetectFilesystem_NoPaths(t *testing.T) {
	pkgs, err := DetectFilesystem(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestDetectFilesystem_DeduplicatesAcrossPaths(t *testing.T) {
	tmp := t.TempDir()
	// Same package in two different bases.
	for _, base := range []string{"base1", "base2"} {
		d := filepath.Join(tmp, base, "python", "3.11.11", "bin")
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Different base paths count as separate entries (dedupe is per base path).
	pkgs, err := DetectFilesystem(context.Background(), []string{
		filepath.Join(tmp, "base1"),
		filepath.Join(tmp, "base2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Errorf("expected 2 (one per base path), got %d", len(pkgs))
	}
}
