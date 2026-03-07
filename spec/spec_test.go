package spec

import (
	"testing"
)

// TestParseSoftwareRef tests all supported software ref string formats.
func TestParseSoftwareRef(t *testing.T) {
	tests := []struct {
		input     string
		want      SoftwareRef
		wantError bool
	}{
		// Basic name
		{
			input: "cuda",
			want:  SoftwareRef{Name: "cuda"},
		},
		// Name with version
		{
			input: "cuda@12.3",
			want:  SoftwareRef{Name: "cuda", Version: "12.3"},
		},
		// Full version
		{
			input: "python@3.11.9",
			want:  SoftwareRef{Name: "python", Version: "3.11.9"},
		},
		// Formation ref
		{
			input: "formation:cuda-python-ml@2024.03",
			want:  SoftwareRef{Formation: "cuda-python-ml@2024.03"},
		},
		// Formation ref without version
		{
			input: "formation:cuda-python-ml",
			want:  SoftwareRef{Formation: "cuda-python-ml"},
		},
		// Errors
		{input: "", wantError: true},
		{input: "formation:", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSoftwareRef(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("ParseSoftwareRef(%q) want error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSoftwareRef(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseSoftwareRef(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

// TestSoftwareRefString tests the String() round-trip.
func TestSoftwareRefString(t *testing.T) {
	tests := []struct {
		ref  SoftwareRef
		want string
	}{
		{SoftwareRef{Name: "cuda"}, "cuda"},
		{SoftwareRef{Name: "cuda", Version: "12.3"}, "cuda@12.3"},
		{SoftwareRef{Formation: "cuda-python-ml@2024.03"}, "formation:cuda-python-ml@2024.03"},
	}

	for _, tt := range tests {
		got := tt.ref.String()
		if got != tt.want {
			t.Errorf("SoftwareRef%+v.String() = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// TestProfileValidation tests Profile validation rules.
func TestProfileValidation(t *testing.T) {
	valid := Profile{
		Name: "test",
		Base: BaseRef{OS: "al2023"},
		Software: []SoftwareRef{
			{Name: "cuda", Version: "12.3"},
		},
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("valid profile failed validation: %v", err)
	}

	// Missing name
	noName := valid
	noName.Name = ""
	if err := noName.Validate(); err == nil {
		t.Error("profile with empty name should fail validation")
	}

	// Missing OS
	noOS := valid
	noOS.Base.OS = ""
	if err := noOS.Validate(); err == nil {
		t.Error("profile with empty base OS should fail validation")
	}

	// Invalid OS
	badOS := valid
	badOS.Base.OS = "windows11"
	if err := badOS.Validate(); err == nil {
		t.Error("profile with invalid OS should fail validation")
	}

	// Empty software list
	noSoftware := valid
	noSoftware.Software = nil
	if err := noSoftware.Validate(); err == nil {
		t.Error("profile with empty software list should fail validation")
	}
}

// TestParseProfileBytes tests YAML profile parsing.
func TestParseProfileBytes(t *testing.T) {
	input := []byte(`
name: alphafold3
base:
  os: al2023
  arch: x86_64
software:
  - formation: cuda-python-ml@2024.03
  - name: alphafold
    version: "3.0"
instance:
  type: p4d.24xlarge
  spot: true
`)

	p, err := ParseProfileBytes(input)
	if err != nil {
		t.Fatalf("ParseProfileBytes() error: %v", err)
	}

	if p.Name != "alphafold3" {
		t.Errorf("Name = %q, want %q", p.Name, "alphafold3")
	}
	if p.Base.OS != "al2023" {
		t.Errorf("Base.OS = %q, want %q", p.Base.OS, "al2023")
	}
	if len(p.Software) != 2 {
		t.Errorf("len(Software) = %d, want 2", len(p.Software))
	}
	if !p.Instance.Spot {
		t.Error("Instance.Spot should be true")
	}
}

// TestBaseRefNormalizedArch tests arch defaulting.
func TestBaseRefNormalizedArch(t *testing.T) {
	empty := BaseRef{OS: "al2023"}
	if empty.NormalizedArch() != "x86_64" {
		t.Errorf("empty arch should default to x86_64, got %q", empty.NormalizedArch())
	}

	arm := BaseRef{OS: "al2023", Arch: "arm64"}
	if arm.NormalizedArch() != "arm64" {
		t.Errorf("arm64 arch should be arm64, got %q", arm.NormalizedArch())
	}
}

// TestRequirementString tests human-readable requirement formatting.
func TestRequirementString(t *testing.T) {
	tests := []struct {
		req  Requirement
		want string
	}{
		{Requirement{Name: "glibc", MinVersion: "2.34"}, "glibc@>=2.34"},
		{Requirement{Name: "cuda", MinVersion: "12.0"}, "cuda@>=12.0"},
		{Requirement{Name: "python", MinVersion: "3.10", MaxVersion: "3.12"}, "python@>=3.10,<3.12"},
		{Requirement{Name: "mpi"}, "mpi"},
	}

	for _, tt := range tests {
		got := tt.req.String()
		if got != tt.want {
			t.Errorf("Requirement%+v.String() = %q, want %q", tt.req, got, tt.want)
		}
	}
}

// TestLockFileIsFrozen tests frozen state detection.
func TestLockFileIsFrozen(t *testing.T) {
	frozen := &LockFile{
		Base: ResolvedBase{AMISHA256: "abc123"},
		Layers: []ResolvedLayer{
			{LayerManifest: LayerManifest{SHA256: "def456"}},
			{LayerManifest: LayerManifest{SHA256: "ghi789"}},
		},
	}
	if !frozen.IsFrozen() {
		t.Error("lockfile with all SHAs should be frozen")
	}

	notFrozen := &LockFile{
		Base: ResolvedBase{}, // no AMI SHA256
		Layers: []ResolvedLayer{
			{LayerManifest: LayerManifest{SHA256: "def456"}},
		},
	}
	if notFrozen.IsFrozen() {
		t.Error("lockfile without AMI SHA256 should not be frozen")
	}
}
