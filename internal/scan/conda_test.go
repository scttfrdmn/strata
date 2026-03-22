package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadCondaEnv(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(file), "testdata")

	// Create a temporary conda-like env pointing at testdata/conda-meta
	tmpEnv := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpEnv, "conda-meta"), 0755); err != nil {
		t.Fatal(err)
	}

	// Copy fixture file
	srcPath := filepath.Join(testdataDir, "conda-meta", "numpy-1.26.4-py311hd.json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	dstPath := filepath.Join(tmpEnv, "conda-meta", "numpy-1.26.4-py311hd.json")
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	pkgs := readCondaEnv(tmpEnv)
	if len(pkgs) != 1 {
		t.Fatalf("got %d packages, want 1", len(pkgs))
	}
	p := pkgs[0]
	if p.Name != "numpy" {
		t.Errorf("Name = %q, want numpy", p.Name)
	}
	if p.Version != "1.26.4" {
		t.Errorf("Version = %q, want 1.26.4", p.Version)
	}
	if p.Channel != "conda-forge" {
		t.Errorf("Channel = %q, want conda-forge", p.Channel)
	}
}
