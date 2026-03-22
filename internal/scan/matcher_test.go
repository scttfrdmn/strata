package scan

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func TestMatchAll(t *testing.T) {
	store := registry.NewMemoryStore()
	store.AddLayer(&spec.LayerManifest{
		Name:    "python",
		Version: "3.11.11",
		Arch:    "x86_64",
		ABI:     "linux-gnu-2.34",
	})
	store.AddLayer(&spec.LayerManifest{
		Name:    "gcc",
		Version: "14.2.0",
		Arch:    "x86_64",
		ABI:     "linux-gnu-2.34",
	})

	pkgs := []DetectedPackage{
		{Name: "python", Version: "3.11.11", Source: SourceLmod},
		{Name: "gcc", Version: "13.2.0", Source: SourceLmod},
		{Name: "mylib", Version: "2.4.1", Source: SourceFilesystem},
	}

	results, err := MatchAll(context.Background(), store, pkgs, "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	if results[0].Status != StatusMatched {
		t.Errorf("python: status = %v, want matched", results[0].Status)
	}
	if results[1].Status != StatusNearMatch {
		t.Errorf("gcc: status = %v, want near_match", results[1].Status)
	}
	if results[1].NearVersion != "14.2.0" {
		t.Errorf("gcc: NearVersion = %q, want 14.2.0", results[1].NearVersion)
	}
	if results[2].Status != StatusUnmatched {
		t.Errorf("mylib: status = %v, want unmatched", results[2].Status)
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"numpy", "numpy"},
		{"NumPy", "numpy"},
		{"scikit_learn", "scikit-learn"},
		{"scikit-learn", "scikit-learn"},
	}
	for _, tt := range tests {
		if got := normalizeName(tt.input); got != tt.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
