package build

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateContentManifest_Basic(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	m, err := GenerateContentManifest(dir, "test-layer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(m.Files))
	}
	for path := range m.Files {
		if len(path) == 0 || path[0] != '/' {
			t.Errorf("path %q must start with /", path)
		}
	}
}

func TestGenerateContentManifest_SHA256Correct(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello, world")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), content, 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	sum := sha256.Sum256(content)
	want := fmt.Sprintf("%x", sum[:])

	m, err := GenerateContentManifest(dir, "test-layer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := m.Files["/hello.txt"]
	if !ok {
		t.Fatal("expected /hello.txt in manifest")
	}
	if got != want {
		t.Errorf("SHA256 mismatch: want %s, got %s", want, got)
	}
}

func TestGenerateContentManifest_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	m, err := GenerateContentManifest(dir, "empty-layer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(m.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(m.Files))
	}
}

func TestGenerateContentManifest_LayerID(t *testing.T) {
	dir := t.TempDir()

	m, err := GenerateContentManifest(dir, "my-layer-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.LayerID != "my-layer-id" {
		t.Errorf("expected LayerID %q, got %q", "my-layer-id", m.LayerID)
	}
}

func TestGenerateContentManifest_Subdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("creating subdirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	m, err := GenerateContentManifest(dir, "test-layer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(m.Files))
	}
	if _, ok := m.Files["/sub/nested/deep.txt"]; !ok {
		t.Errorf("expected /sub/nested/deep.txt in manifest, got: %v", m.Files)
	}
}
