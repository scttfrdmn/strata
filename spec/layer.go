package spec

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LayerManifest is what the registry knows about a layer.
// It is produced by the build pipeline and never hand-written.
// Every field is populated before the layer enters the registry.
type LayerManifest struct {
	// Identity
	ID      string `yaml:"id" json:"id"`           // e.g. "python-3.11.9-linux-gnu-2.34-x86_64"
	Name    string `yaml:"name" json:"name"`       // e.g. "python"
	Version string `yaml:"version" json:"version"` // e.g. "3.11.9"

	// Content
	Source string `yaml:"source" json:"source"` // s3://strata-layers/...
	SHA256 string `yaml:"sha256" json:"sha256"` // hex SHA256 of squashfs
	Size   int64  `yaml:"size" json:"size"`     // bytes

	// ContentManifest maps file path to SHA256 of file content for every
	// file in the layer. Used for conflict detection at resolve time.
	ContentManifest map[string]string `yaml:"content_manifest" json:"content_manifest"`

	// Sigstore attestation — mandatory, populated at build time.
	RekorEntry    string `yaml:"rekor_entry" json:"rekor_entry"`       // Rekor log entry ID
	Bundle        string `yaml:"bundle" json:"bundle"`                 // cosign bundle path in registry
	SignedBy      string `yaml:"signed_by" json:"signed_by"`           // signing key identity
	CosignVersion string `yaml:"cosign_version" json:"cosign_version"` // e.g. "v3.0.5"

	// Capability contract — what this layer provides and requires.
	Provides []Capability  `yaml:"provides" json:"provides"`
	Requires []Requirement `yaml:"requires" json:"requires"`

	// Registry metadata.
	Arch    string    `yaml:"arch" json:"arch"` // "x86_64", "arm64"
	ABI     string    `yaml:"abi" json:"abi"`   // e.g. "linux-gnu-2.34", "linux-gnu-2.35"
	BuiltAt time.Time `yaml:"built_at" json:"built_at"`

	// UserSelectable is false for dependency-only layers (ucx, hwloc, pmix, libfabric)
	// that are resolved transitively but never shown in default strata search output
	// or allowed as top-level user software choices.
	UserSelectable bool `yaml:"user_selectable" json:"user_selectable"`

	// InstallLayout describes how the recipe installs into the squashfs.
	// "versioned" (default): installs to /<name>/<version>/
	// "flat": installs directly to / (used by glibc, musl)
	InstallLayout string `yaml:"install_layout,omitempty" json:"install_layout,omitempty"`

	// HasModulefile is true when the build pipeline generated an Lmod
	// modulefile (modulefiles/<name>/<version>.lua) inside the squashfs.
	// Set for all non-flat layouts by the build pipeline after a successful
	// GenerateModulefile call.
	HasModulefile bool `yaml:"has_modulefile,omitempty" json:"has_modulefile,omitempty"`

	// Build provenance — what produced this layer.
	RecipeSHA256      string     `yaml:"recipe_sha256" json:"recipe_sha256"`                               // SHA256 of build.sh
	BuildEnvLockID    string     `yaml:"build_env_lock_id" json:"build_env_lock_id"`                       // SHA256 of BaseCapabilities probe (bootstrap) or resolved layer lockfile
	BuiltWith         []LayerRef `yaml:"built_with,omitempty" json:"built_with,omitempty"`                 // Strata layers that formed the build env (Tier 0.5+)
	BootstrapBuild    bool       `yaml:"bootstrap_build,omitempty" json:"bootstrap_build,omitempty"`       // true = Tier 0, built with OS system compiler
	BootstrapCompiler string     `yaml:"bootstrap_compiler,omitempty" json:"bootstrap_compiler,omitempty"` // exact system compiler package, e.g. "gcc-11.4.1-2.amzn2023.0.1.x86_64"

	// CaptureSource records how this layer was created without a recipe.
	// Values: "lmod", "conda", "filesystem", "fold". Empty for recipe-built layers.
	CaptureSource string `yaml:"capture_source,omitempty" json:"capture_source,omitempty"`

	// FoldedFrom lists SHA256 hashes of the source layers that were merged
	// to produce this layer. Populated only for layers created by strata fold.
	FoldedFrom []string `yaml:"folded_from,omitempty" json:"folded_from,omitempty"`

	// OriginalPrefix is the install root from which the layer was captured.
	OriginalPrefix string `yaml:"original_prefix,omitempty" json:"original_prefix,omitempty"`

	// Normalized is true if absolute paths were rewritten during capture.
	Normalized bool `yaml:"normalized,omitempty" json:"normalized,omitempty"`
}

// LayerRef is a compact reference to a specific layer used in a build environment.
// It records enough information to independently verify the build provenance via Rekor.
type LayerRef struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	SHA256  string `yaml:"sha256" json:"sha256"`
	Rekor   string `yaml:"rekor_entry" json:"rekor_entry"`
}

// Capability is a named, versioned capability that a layer provides or requires.
// The version string is a semver-compatible value.
//
// Examples:
//
//	{Name: "python",  Version: "3.11.9"}
//	{Name: "cuda",    Version: "12.3.2"}
//	{Name: "glibc",   Version: "2.34"}
//	{Name: "mpi",     Version: "3.1"}
//	{Name: "abi",     Version: "linux-gnu-2.34"}  // C runtime ABI identifier
type Capability struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// String returns "name@version".
func (c Capability) String() string {
	if c.Version == "" {
		return c.Name
	}
	return c.Name + "@" + c.Version
}

// Requirement is a capability requirement with optional version bounds.
// MinVersion and MaxVersion are semver strings.
// An empty bound means unbounded in that direction.
//
// Examples:
//
//	{Name: "glibc",  MinVersion: "2.34"}                       // glibc >= 2.34
//	{Name: "cuda",   MinVersion: "12.0"}                       // cuda >= 12.0
//	{Name: "python", MinVersion: "3.10", MaxVersion: "3.12"}   // 3.10 <= python < 3.12
type Requirement struct {
	Name       string `yaml:"name" json:"name"`
	MinVersion string `yaml:"min_version,omitempty" json:"min_version,omitempty"`
	MaxVersion string `yaml:"max_version,omitempty" json:"max_version,omitempty"`
}

// String returns a human-readable representation of the requirement.
func (r Requirement) String() string {
	switch {
	case r.MinVersion != "" && r.MaxVersion != "":
		return fmt.Sprintf("%s@>=%s,<%s", r.Name, r.MinVersion, r.MaxVersion)
	case r.MinVersion != "":
		return fmt.Sprintf("%s@>=%s", r.Name, r.MinVersion)
	case r.MaxVersion != "":
		return fmt.Sprintf("%s@<%s", r.Name, r.MaxVersion)
	default:
		return r.Name
	}
}

// BaseCapabilities describes the capabilities of a base AMI.
// Produced by the probe pipeline, cached by AMI ID. Never hand-written.
type BaseCapabilities struct {
	// AMIID is the EC2 AMI ID this probe was run against.
	AMIID string `yaml:"ami_id" json:"ami_id"`

	// OS is the declared OS identifier, e.g. "al2023".
	OS string `yaml:"os" json:"os"`

	// Arch is the architecture, e.g. "x86_64".
	Arch string `yaml:"arch" json:"arch"`

	// ABI is the C runtime ABI identifier used for layer catalog filtering.
	// "linux-gnu-2.34" covers AL2023, Rocky 9/10, RHEL 9.
	// "linux-gnu-2.35" covers Ubuntu 22.04, Debian 12.
	ABI string `yaml:"abi" json:"abi"`

	// ProbedAt is when this capability set was generated.
	ProbedAt time.Time `yaml:"probed_at" json:"probed_at"`

	// SystemCompiler is the exact package identifier of the system C compiler.
	// Populated by the probe script; format is ABI-specific but always
	// uniquely identifies the compiler version and build:
	//   linux-gnu (rpm):  rpm NVR, e.g. "gcc-11.4.1-2.amzn2023.0.1.x86_64"
	//   linux-gnu (dpkg): dpkg NVR, e.g. "gcc-11-11.4.0-1ubuntu1~22.04-amd64"
	// Used as LayerManifest.BootstrapCompiler for Tier 0 bootstrap builds.
	SystemCompiler string `yaml:"system_compiler" json:"system_compiler"`

	// Provides is the list of capabilities the base OS provides.
	// Populated by the probe script running on the actual AMI.
	// Includes: glibc, kernel, systemd, rpm/dpkg version, compiler basics.
	Provides []Capability `yaml:"provides" json:"provides"`
}

// HasCapability reports whether the base provides the named capability
// at or above the specified minimum version.
// An empty minVersion means any version satisfies.
func (b BaseCapabilities) HasCapability(name, minVersion string) bool {
	for _, cap := range b.Provides {
		if cap.Name == name {
			if minVersion == "" {
				return true
			}
			return semverGTE(cap.Version, minVersion)
		}
	}
	return false
}

// SatisfiesRequirement reports whether the base satisfies a Requirement.
func (b BaseCapabilities) SatisfiesRequirement(r Requirement) bool {
	for _, cap := range b.Provides {
		if cap.Name == r.Name {
			if r.MinVersion != "" && !semverGTE(cap.Version, r.MinVersion) {
				return false
			}
			if r.MaxVersion != "" && !semverLT(cap.Version, r.MaxVersion) {
				return false
			}
			return true
		}
	}
	return false
}

// Formation is a named, versioned, pre-validated group of layers.
// Formations are conflict-checked and smoke-tested as a unit before entering
// the registry. They are the primary way users compose multi-layer environments.
type Formation struct {
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Layers is the ordered list of software refs in this formation.
	// These are resolved to specific LayerManifests at formation build time.
	Layers []SoftwareRef `yaml:"layers" json:"layers"`

	// Provides is the union of capabilities provided by all layers.
	// Computed at formation build time.
	Provides []Capability `yaml:"provides" json:"provides"`

	// ValidatedOn lists the base OS/arch combinations this formation
	// has been smoke-tested against, e.g. ["al2023/x86_64", "al2023/arm64"].
	ValidatedOn []string `yaml:"validated_on" json:"validated_on"`

	// Sigstore attestation — the formation is signed as a unit.
	RekorEntry string `yaml:"rekor_entry" json:"rekor_entry"`
	Bundle     string `yaml:"bundle" json:"bundle"`
	SignedBy   string `yaml:"signed_by" json:"signed_by"`

	BuiltAt time.Time `yaml:"built_at" json:"built_at"`
}

// semverGTE reports whether version a >= version b.
// Versions are compared numerically segment-by-segment on "."-split fields,
// so "3.11" > "3.9" and "12.3.2" >= "12.3". Non-numeric segments fall back
// to lexicographic comparison. A leading "v" is stripped if present.
func semverGTE(a, b string) bool {
	return compareVersions(a, b) >= 0
}

// semverLT reports whether version a < version b.
func semverLT(a, b string) bool {
	return compareVersions(a, b) < 0
}

// compareVersions compares two dotted version strings numerically.
// Returns -1, 0, or 1 analogous to strings.Compare.
func compareVersions(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := range n {
		var av, bv int
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
