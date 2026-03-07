package spec

import (
	"os"
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

// TestParseProfileFile tests file-based profile parsing.
func TestParseProfileFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "profile-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("name: test\nbase:\n  os: al2023\nsoftware:\n  - name: cuda\n    version: \"12.3\"\n")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	p, err := ParseProfile(f.Name())
	if err != nil {
		t.Fatalf("ParseProfile() error: %v", err)
	}
	if p.Name != "test" {
		t.Errorf("Name = %q, want %q", p.Name, "test")
	}

	_, err = ParseProfile(f.Name() + ".missing")
	if err == nil {
		t.Error("ParseProfile on missing file should return error")
	}
}

// TestParseLockFileFile tests file-based lockfile parsing.
func TestParseLockFileFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "lockfile-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("profile: test\nstrata_version: 0.1.0\n")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	l, err := ParseLockFile(f.Name())
	if err != nil {
		t.Fatalf("ParseLockFile() error: %v", err)
	}
	if l.ProfileName != "test" {
		t.Errorf("ProfileName = %q, want %q", l.ProfileName, "test")
	}

	_, err = ParseLockFile(f.Name() + ".missing")
	if err == nil {
		t.Error("ParseLockFile on missing file should return error")
	}
}

// TestMarshalProfile tests Profile marshaling.
func TestMarshalProfile(t *testing.T) {
	p := &Profile{
		Name:     "test",
		Base:     BaseRef{OS: "al2023"},
		Software: []SoftwareRef{{Name: "cuda", Version: "12.3"}},
	}
	data, err := MarshalProfile(p)
	if err != nil {
		t.Fatalf("MarshalProfile() error: %v", err)
	}
	p2, err := ParseProfileBytes(data)
	if err != nil {
		t.Fatalf("ParseProfileBytes() after marshal error: %v", err)
	}
	if p.Name != p2.Name || p.Software[0].String() != p2.Software[0].String() {
		t.Errorf("marshal round-trip mismatch: %+v != %+v", p, p2)
	}
}

// TestSoftwareRefValidateInvalidName tests that invalid name characters are rejected.
func TestSoftwareRefValidateInvalidName(t *testing.T) {
	invalidNames := []string{"cuda 12", "cuda@bad", "cu:da", "cu/da"}
	for _, name := range invalidNames {
		ref := SoftwareRef{Name: name}
		if err := ref.Validate(); err == nil {
			t.Errorf("SoftwareRef{Name: %q}.Validate() should return error", name)
		}
	}
}

// TestCapabilityString tests Capability.String().
func TestCapabilityString(t *testing.T) {
	tests := []struct {
		cap  Capability
		want string
	}{
		{Capability{Name: "python", Version: "3.11.9"}, "python@3.11.9"},
		{Capability{Name: "mpi", Version: ""}, "mpi"},
	}
	for _, tt := range tests {
		if got := tt.cap.String(); got != tt.want {
			t.Errorf("Capability%+v.String() = %q, want %q", tt.cap, got, tt.want)
		}
	}
}

// TestHasCapability tests BaseCapabilities.HasCapability.
func TestHasCapability(t *testing.T) {
	base := BaseCapabilities{
		Provides: []Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "python", Version: "3.11.9"},
		},
	}

	if !base.HasCapability("glibc", "") {
		t.Error("HasCapability(glibc, '') should be true (any version)")
	}
	if !base.HasCapability("glibc", "2.34") {
		t.Error("HasCapability(glibc, 2.34) should be true (exact match)")
	}
	if !base.HasCapability("glibc", "2.33") {
		t.Error("HasCapability(glibc, 2.33) should be true (installed version is newer)")
	}
	if base.HasCapability("glibc", "2.35") {
		t.Error("HasCapability(glibc, 2.35) should be false (installed version is older)")
	}
	if base.HasCapability("cuda", "") {
		t.Error("HasCapability(cuda, '') should be false (not present)")
	}
}

// TestParseLockFileBytes tests LockFile YAML parsing.
func TestParseLockFileBytes(t *testing.T) {
	input := []byte(`
profile: alphafold3
profile_sha256: abc123
strata_version: 0.1.0
base:
  declared_os: al2023
  ami_id: ami-0abc123
  ami_sha256: def456
layers:
  - id: python-3.11.9-rhel-x86_64
    name: python
    version: 3.11.9
    sha256: aaa111
    mount_order: 1
    satisfied_by: "formation:cuda-python-ml@2024.03"
`)
	l, err := ParseLockFileBytes(input)
	if err != nil {
		t.Fatalf("ParseLockFileBytes() error: %v", err)
	}
	if l.ProfileName != "alphafold3" {
		t.Errorf("ProfileName = %q, want %q", l.ProfileName, "alphafold3")
	}
	if l.Base.AMIID != "ami-0abc123" {
		t.Errorf("Base.AMIID = %q, want %q", l.Base.AMIID, "ami-0abc123")
	}
	if len(l.Layers) != 1 {
		t.Fatalf("len(Layers) = %d, want 1", len(l.Layers))
	}
	if l.Layers[0].Name != "python" {
		t.Errorf("Layers[0].Name = %q, want %q", l.Layers[0].Name, "python")
	}
}

// TestLockFileHelpers tests IsSigned, LayerCount, and ProvenanceRecord.
func TestLockFileHelpers(t *testing.T) {
	l := &LockFile{
		ProfileName:   "test",
		ProfileSHA256: "sha-profile",
		StrataVersion: "0.1.0",
		Base:          ResolvedBase{AMIID: "ami-abc", AMISHA256: "ami-sha"},
		Layers: []ResolvedLayer{
			{LayerManifest: LayerManifest{Name: "python", Version: "3.11", SHA256: "sha1", RekorEntry: "rekor1"}, MountOrder: 1},
			{LayerManifest: LayerManifest{Name: "cuda", Version: "12.3", SHA256: "sha2", RekorEntry: "rekor2"}, MountOrder: 2},
		},
	}

	if l.IsSigned() {
		t.Error("unsigned lockfile should not be signed")
	}
	l.RekorEntry = "lockfile-rekor"
	if !l.IsSigned() {
		t.Error("lockfile with RekorEntry should be signed")
	}

	if l.LayerCount() != 2 {
		t.Errorf("LayerCount() = %d, want 2", l.LayerCount())
	}

	prov := l.ProvenanceRecord()
	if prov.Profile != "test" {
		t.Errorf("ProvenanceRecord.Profile = %q, want %q", prov.Profile, "test")
	}
	if prov.AMIID != "ami-abc" {
		t.Errorf("ProvenanceRecord.AMIID = %q, want %q", prov.AMIID, "ami-abc")
	}
	if len(prov.Layers) != 2 {
		t.Errorf("ProvenanceRecord.Layers count = %d, want 2", len(prov.Layers))
	}
	if prov.Layers[0].Name != "python" {
		t.Errorf("ProvenanceRecord.Layers[0].Name = %q, want %q", prov.Layers[0].Name, "python")
	}
	if prov.LockfileRekor != "lockfile-rekor" {
		t.Errorf("ProvenanceRecord.LockfileRekor = %q, want %q", prov.LockfileRekor, "lockfile-rekor")
	}
}

// TestMarshalLockFile tests LockFile marshaling round-trip.
func TestMarshalLockFile(t *testing.T) {
	l := &LockFile{
		ProfileName:   "test",
		StrataVersion: "0.1.0",
		Base:          ResolvedBase{DeclaredOS: "al2023", AMIID: "ami-abc"},
		Layers: []ResolvedLayer{
			{LayerManifest: LayerManifest{Name: "python", Version: "3.11", SHA256: "sha1"}, MountOrder: 1},
		},
	}
	data, err := MarshalLockFile(l)
	if err != nil {
		t.Fatalf("MarshalLockFile() error: %v", err)
	}
	l2, err := ParseLockFileBytes(data)
	if err != nil {
		t.Fatalf("ParseLockFileBytes() after marshal error: %v", err)
	}
	if l.ProfileName != l2.ProfileName {
		t.Errorf("round-trip ProfileName: %q != %q", l.ProfileName, l2.ProfileName)
	}
	if l.Layers[0].SHA256 != l2.Layers[0].SHA256 {
		t.Errorf("round-trip Layers[0].SHA256: %q != %q", l.Layers[0].SHA256, l2.Layers[0].SHA256)
	}
}

// TestNormalizeSoftwareRefsInlineFormation tests that an inline
// "formation:foo" string in the name field is correctly re-parsed.
func TestNormalizeSoftwareRefsInlineFormation(t *testing.T) {
	input := []byte(`
name: test
base:
  os: al2023
software:
  - name: "formation:cuda-python-ml@2024.03"
`)
	p, err := ParseProfileBytes(input)
	if err != nil {
		t.Fatalf("ParseProfileBytes() error: %v", err)
	}
	if len(p.Software) != 1 {
		t.Fatalf("len(Software) = %d, want 1", len(p.Software))
	}
	if !p.Software[0].IsFormation() {
		t.Errorf("software[0] should be a formation ref after normalization, got %+v", p.Software[0])
	}
	if p.Software[0].Formation != "cuda-python-ml@2024.03" {
		t.Errorf("Formation = %q, want %q", p.Software[0].Formation, "cuda-python-ml@2024.03")
	}
}

// TestSemverComparison tests numeric segment-wise version comparison.
func TestSemverComparison(t *testing.T) {
	tests := []struct {
		a, b    string
		wantGTE bool
		wantLT  bool
	}{
		// Basic ordering
		{"3.11", "3.9", true, false},  // 11 > 9 numerically
		{"3.9", "3.11", false, true},  // string compare would get this wrong
		{"12.3", "12.3", true, false}, // equal
		{"12.4", "12.3", true, false},
		{"12.3", "12.4", false, true},
		// Three-part versions
		{"3.11.9", "3.11.8", true, false},
		{"3.11.9", "3.11.9", true, false},
		{"3.11.8", "3.11.9", false, true},
		// Different segment counts — shorter is padded with 0
		{"3.11", "3.11.0", true, false},
		{"3.11.1", "3.11", true, false},
		// Leading v stripped
		{"v2.34", "2.34", true, false},
		{"2.34", "v2.33", true, false},
		// glibc-style
		{"2.34", "2.34", true, false},
		{"2.35", "2.34", true, false},
		{"2.33", "2.34", false, true},
	}

	for _, tt := range tests {
		if got := semverGTE(tt.a, tt.b); got != tt.wantGTE {
			t.Errorf("semverGTE(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.wantGTE)
		}
		if got := semverLT(tt.a, tt.b); got != tt.wantLT {
			t.Errorf("semverLT(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.wantLT)
		}
	}
}

// TestSatisfiesRequirement tests BaseCapabilities requirement matching.
func TestSatisfiesRequirement(t *testing.T) {
	base := BaseCapabilities{
		Provides: []Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "python", Version: "3.11.9"},
			{Name: "kernel", Version: "5.15"},
		},
	}

	tests := []struct {
		req  Requirement
		want bool
	}{
		{Requirement{Name: "glibc", MinVersion: "2.34"}, true},
		{Requirement{Name: "glibc", MinVersion: "2.35"}, false},
		{Requirement{Name: "glibc", MinVersion: "2.33"}, true},
		{Requirement{Name: "python", MinVersion: "3.10", MaxVersion: "3.12"}, true},
		{Requirement{Name: "python", MinVersion: "3.12"}, false},
		{Requirement{Name: "python", MaxVersion: "3.11"}, false}, // 3.11.9 >= 3.11, not < 3.11
		{Requirement{Name: "cuda"}, false},                       // not present
		{Requirement{Name: "kernel"}, true},                      // no version constraint
	}

	for _, tt := range tests {
		if got := base.SatisfiesRequirement(tt.req); got != tt.want {
			t.Errorf("SatisfiesRequirement(%v) = %v, want %v", tt.req, got, tt.want)
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

// TestEnvironmentID tests that EnvironmentID produces stable, content-based hashes.
func TestEnvironmentID(t *testing.T) {
	base := ResolvedBase{AMISHA256: "aaaaaa"}
	layers := []ResolvedLayer{
		{LayerManifest: LayerManifest{SHA256: "bbbbbb"}, MountOrder: 1},
		{LayerManifest: LayerManifest{SHA256: "cccccc"}, MountOrder: 2},
	}

	l := &LockFile{Base: base, Layers: layers}
	id := l.EnvironmentID()
	if id == "" {
		t.Fatal("frozen lockfile should produce non-empty EnvironmentID")
	}
	if len(id) != 64 {
		t.Errorf("EnvironmentID should be 64 hex chars, got %d: %q", len(id), id)
	}

	// Same content → same ID.
	l2 := &LockFile{Base: base, Layers: layers}
	if l.EnvironmentID() != l2.EnvironmentID() {
		t.Error("identical lockfiles should produce identical EnvironmentIDs")
	}

	// Different layer SHA256 → different ID.
	otherLayers := []ResolvedLayer{
		{LayerManifest: LayerManifest{SHA256: "bbbbbb"}, MountOrder: 1},
		{LayerManifest: LayerManifest{SHA256: "dddddd"}, MountOrder: 2},
	}
	l3 := &LockFile{Base: base, Layers: otherLayers}
	if l.EnvironmentID() == l3.EnvironmentID() {
		t.Error("lockfiles with different layer SHA256s should produce different EnvironmentIDs")
	}

	// Attestation-only change (RekorEntry) does not change ID.
	lSigned := &LockFile{Base: base, Layers: layers, RekorEntry: "rekor-xyz"}
	if l.EnvironmentID() != lSigned.EnvironmentID() {
		t.Error("signing (RekorEntry) should not change EnvironmentID")
	}

	// Non-frozen lockfile returns empty string.
	notFrozen := &LockFile{
		Base:   ResolvedBase{},
		Layers: layers,
	}
	if notFrozen.EnvironmentID() != "" {
		t.Error("non-frozen lockfile should return empty EnvironmentID")
	}

	// Layer order by MountOrder should be deterministic.
	reversedLayers := []ResolvedLayer{layers[1], layers[0]} // same content, reversed in slice
	lReversed := &LockFile{Base: base, Layers: reversedLayers}
	if l.EnvironmentID() != lReversed.EnvironmentID() {
		t.Error("layer slice order should not affect EnvironmentID — only MountOrder matters")
	}
}
