package scan

import (
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func TestProfileFromMatches_OnlyIncludesMatched(t *testing.T) {
	results := []MatchResult{
		{Status: StatusMatched, Package: DetectedPackage{Name: "python", Version: "3.11.11"}},
		{Status: StatusUnmatched, Package: DetectedPackage{Name: "unknown-lib", Version: "1.0"}},
		{Status: StatusMatched, Package: DetectedPackage{Name: "gcc", Version: "13.2.0"}},
	}
	p := ProfileFromMatches(results, "test-profile", "al2023", "x86_64")

	if p.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", p.Name, "test-profile")
	}
	if p.Base.OS != "al2023" {
		t.Errorf("Base.OS = %q, want %q", p.Base.OS, "al2023")
	}
	if p.Base.Arch != "x86_64" {
		t.Errorf("Base.Arch = %q, want %q", p.Base.Arch, "x86_64")
	}
	if len(p.Software) != 2 {
		t.Fatalf("len(Software) = %d, want 2", len(p.Software))
	}
	if p.Software[0].Name != "python" || p.Software[0].Version != "3.11.11" {
		t.Errorf("Software[0] = %+v, want python@3.11.11", p.Software[0])
	}
	if p.Software[1].Name != "gcc" || p.Software[1].Version != "13.2.0" {
		t.Errorf("Software[1] = %+v, want gcc@13.2.0", p.Software[1])
	}
}

func TestProfileFromMatches_Empty(t *testing.T) {
	p := ProfileFromMatches(nil, "empty", "al2023", "x86_64")
	if len(p.Software) != 0 {
		t.Errorf("expected no software, got %d", len(p.Software))
	}
}

func TestProfileFromMatches_AllUnmatched(t *testing.T) {
	results := []MatchResult{
		{Status: StatusUnmatched, Package: DetectedPackage{Name: "mystery", Version: "1.0"}},
	}
	p := ProfileFromMatches(results, "empty-profile", "al2023", "x86_64")
	if len(p.Software) != 0 {
		t.Errorf("expected no software, got %d", len(p.Software))
	}
}

func TestLockFileFromMatches_IncludesOnlyMatchedWithManifest(t *testing.T) {
	manifest1 := &spec.LayerManifest{Name: "python", Version: "3.11.11", ID: "python-3.11.11"}
	manifest2 := &spec.LayerManifest{Name: "gcc", Version: "13.2.0", ID: "gcc-13.2.0"}

	results := []MatchResult{
		{Status: StatusMatched, Package: DetectedPackage{Name: "python", Version: "3.11.11"}, Manifest: manifest1},
		{Status: StatusUnmatched, Package: DetectedPackage{Name: "unknown", Version: "1.0"}, Manifest: nil},
		{Status: StatusMatched, Package: DetectedPackage{Name: "gcc", Version: "13.2.0"}, Manifest: manifest2},
		// matched but nil manifest — should be skipped
		{Status: StatusMatched, Package: DetectedPackage{Name: "orphan", Version: "2.0"}, Manifest: nil},
	}

	lf := LockFileFromMatches(results, "test", "al2023", "x86_64", "linux-gnu-2.34")

	if lf.ProfileName != "test" {
		t.Errorf("ProfileName = %q, want %q", lf.ProfileName, "test")
	}
	if lf.Base.DeclaredOS != "al2023" {
		t.Errorf("Base.DeclaredOS = %q, want %q", lf.Base.DeclaredOS, "al2023")
	}
	if lf.Base.Capabilities.ABI != "linux-gnu-2.34" {
		t.Errorf("Capabilities.ABI = %q, want %q", lf.Base.Capabilities.ABI, "linux-gnu-2.34")
	}
	if len(lf.Layers) != 2 {
		t.Fatalf("len(Layers) = %d, want 2", len(lf.Layers))
	}
	if lf.Layers[0].Name != "python" {
		t.Errorf("Layers[0].Name = %q, want python", lf.Layers[0].Name)
	}
	if lf.Layers[1].Name != "gcc" {
		t.Errorf("Layers[1].Name = %q, want gcc", lf.Layers[1].Name)
	}
}

func TestLockFileFromMatches_MountOrderIsIndex(t *testing.T) {
	m := &spec.LayerManifest{Name: "python", Version: "3.11.11"}
	results := []MatchResult{
		{Status: StatusUnmatched},
		{Status: StatusMatched, Manifest: m},
	}
	lf := LockFileFromMatches(results, "p", "al2023", "x86_64", "linux-gnu-2.34")
	if len(lf.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(lf.Layers))
	}
	// MountOrder = index in results slice (1, not 0)
	if lf.Layers[0].MountOrder != 1 {
		t.Errorf("MountOrder = %d, want 1", lf.Layers[0].MountOrder)
	}
}
