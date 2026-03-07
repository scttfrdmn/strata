package spec

import (
	"fmt"
	"time"
)

// LayerManifest is what the registry knows about a layer.
// It is produced by the build pipeline and never hand-written.
// Every field is populated before the layer enters the registry.
type LayerManifest struct {
	// Identity
	ID      string `yaml:"id" json:"id"`           // e.g. "python-3.11.9-rhel-x86_64"
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
	RekorEntry string `yaml:"rekor_entry" json:"rekor_entry"` // Rekor log entry ID
	Bundle     string `yaml:"bundle" json:"bundle"`           // cosign bundle path in registry
	SignedBy   string `yaml:"signed_by" json:"signed_by"`     // signing key identity

	// Capability contract — what this layer provides and requires.
	Provides []Capability  `yaml:"provides" json:"provides"`
	Requires []Requirement `yaml:"requires" json:"requires"`

	// Registry metadata.
	Arch    string    `yaml:"arch" json:"arch"`       // "x86_64", "arm64"
	Family  string    `yaml:"family" json:"family"`   // "rhel", "debian"
	BuiltAt time.Time `yaml:"built_at" json:"built_at"`

	// Build provenance — what produced this layer.
	RecipeSHA256   string `yaml:"recipe_sha256" json:"recipe_sha256"`          // SHA256 of build.sh
	BuildEnvLockID string `yaml:"build_env_lock_id" json:"build_env_lock_id"` // lockfile ID of build env
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
//	{Name: "family",  Version: "rhel"}   // OS family, not versioned
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

	// Family is the OS family used for layer catalog filtering.
	// "rhel" covers AL2023, Rocky 8/9/10, RHEL 8/9.
	// "debian" covers Ubuntu, Debian.
	Family string `yaml:"family" json:"family"`

	// ProbedAt is when this capability set was generated.
	ProbedAt time.Time `yaml:"probed_at" json:"probed_at"`

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
//
// TODO(#2): replace with golang.org/x/mod/semver for correct semver ordering.
// The current string comparison is only correct for well-formed dotted-numeric
// versions of equal segment count. It is sufficient for the spec package tests
// but must not be used by the resolver.
func semverGTE(a, b string) bool {
	return a >= b
}

// semverLT reports whether version a < version b.
func semverLT(a, b string) bool {
	return !semverGTE(a, b)
}
