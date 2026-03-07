// Package spec defines the core types for Strata profiles, layers, formations,
// and lockfiles. These types are the contract between users, the registry,
// the resolver, and the agent. All other Strata components are built on these.
package spec

import (
	"fmt"
	"strings"
)

// Profile is what users write. A declaration of intent: which software,
// on which base OS, on which instance type. The resolver transforms a
// Profile into a LockFile.
type Profile struct {
	// Name is a short identifier for this environment.
	Name string `yaml:"name" json:"name"`

	// Description is optional human-readable context.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Version is the profile version, for change tracking.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Base declares the target OS and architecture.
	Base BaseRef `yaml:"base" json:"base"`

	// Software is the list of software refs the user wants.
	// Entries may be individual software refs ("cuda@12.3")
	// or formation refs ("formation:cuda-python-ml@2024.03").
	Software []SoftwareRef `yaml:"software" json:"software"`

	// Registries declares which layer registries to use and trust.
	// If empty, the default Strata public registry is used.
	Registries []RegistryRef `yaml:"registries,omitempty" json:"registries,omitempty"`

	// Instance describes the target EC2 instance configuration.
	Instance InstanceConfig `yaml:"instance,omitempty" json:"instance,omitempty"`

	// Storage describes additional storage mounts.
	Storage []StorageMount `yaml:"storage,omitempty" json:"storage,omitempty"`

	// Env declares environment variables set in the assembled environment.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// OnReady is a list of commands run after the overlay is assembled,
	// before the instance signals ready.
	OnReady []string `yaml:"on_ready,omitempty" json:"on_ready,omitempty"`
}

// BaseRef identifies the target OS and architecture for environment assembly.
// The OS string is resolved to an AMI ID via SSM parameter lookup or an
// alias table. The resolved AMI is then probed for its capabilities.
type BaseRef struct {
	// OS is the operating system identifier.
	// Supported: "al2023", "rocky9", "rocky10", "ubuntu24"
	OS string `yaml:"os" json:"os"`

	// Arch is the target architecture.
	// Supported: "x86_64" (default), "arm64"
	Arch string `yaml:"arch,omitempty" json:"arch,omitempty"`
}

// NormalizedArch returns the architecture string, defaulting to x86_64.
func (b BaseRef) NormalizedArch() string {
	if b.Arch == "" {
		return "x86_64"
	}
	return b.Arch
}

// Validate checks that a BaseRef is well-formed.
func (b BaseRef) Validate() error {
	validOS := map[string]bool{
		"al2023":   true,
		"rocky9":   true,
		"rocky10":  true,
		"ubuntu24": true,
	}
	validArch := map[string]bool{
		"x86_64": true,
		"arm64":  true,
		"":       true, // empty is valid (defaults to x86_64)
	}
	if !validOS[b.OS] {
		return fmt.Errorf("unsupported OS %q — supported: al2023, rocky9, rocky10, ubuntu24", b.OS)
	}
	if !validArch[b.Arch] {
		return fmt.Errorf("unsupported arch %q — supported: x86_64, arm64", b.Arch)
	}
	return nil
}

// SoftwareRef is a user-declared software requirement.
// It is either a named software package ("cuda@12.3") or a formation
// reference ("formation:cuda-python-ml@2024.03"). The two forms are
// mutually exclusive.
type SoftwareRef struct {
	// Name is the software name for individual package refs.
	// Empty when Formation is set.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Version is a semver prefix constraint for individual package refs.
	// "12.3" matches "12.3.0", "12.3.1", "12.3.2", etc.
	// Empty means "latest stable".
	// Empty when Formation is set.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Formation is the formation identifier for formation refs.
	// Format: "name@version" e.g. "cuda-python-ml@2024.03"
	// Empty when Name is set.
	Formation string `yaml:"formation,omitempty" json:"formation,omitempty"`
}

// IsFormation reports whether this ref is a formation reference.
func (s SoftwareRef) IsFormation() bool {
	return s.Formation != ""
}

// String returns the canonical string representation.
// "cuda@12.3", "cuda" (no version), "formation:cuda-python-ml@2024.03"
func (s SoftwareRef) String() string {
	if s.IsFormation() {
		return "formation:" + s.Formation
	}
	if s.Version != "" {
		return s.Name + "@" + s.Version
	}
	return s.Name
}

// Validate checks that a SoftwareRef is well-formed.
func (s SoftwareRef) Validate() error {
	if s.IsFormation() && s.Name != "" {
		return fmt.Errorf("software ref cannot have both formation and name set")
	}
	if !s.IsFormation() && s.Name == "" {
		return fmt.Errorf("software ref must have either name or formation set")
	}
	if s.Name != "" && strings.ContainsAny(s.Name, " \t\n@:/") {
		return fmt.Errorf("invalid software name %q", s.Name)
	}
	return nil
}

// ParseSoftwareRef parses a software ref string into a SoftwareRef.
//
// Accepted formats:
//
//	"cuda"                             → {Name: "cuda"}
//	"cuda@12.3"                        → {Name: "cuda", Version: "12.3"}
//	"formation:cuda-python-ml@2024.03" → {Formation: "cuda-python-ml@2024.03"}
func ParseSoftwareRef(s string) (SoftwareRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return SoftwareRef{}, fmt.Errorf("empty software ref")
	}

	// Formation ref
	if strings.HasPrefix(s, "formation:") {
		formation := strings.TrimPrefix(s, "formation:")
		if formation == "" {
			return SoftwareRef{}, fmt.Errorf("empty formation name in ref %q", s)
		}
		return SoftwareRef{Formation: formation}, nil
	}

	// Package ref — split on @ for version
	parts := strings.SplitN(s, "@", 2)
	ref := SoftwareRef{Name: parts[0]}
	if len(parts) == 2 {
		ref.Version = parts[1]
	}

	if err := ref.Validate(); err != nil {
		return SoftwareRef{}, fmt.Errorf("invalid software ref %q: %w", s, err)
	}

	return ref, nil
}

// RegistryRef declares a layer registry and how to trust it.
type RegistryRef struct {
	// URL is the S3 URL of the registry, e.g. "s3://strata-public-layers".
	URL string `yaml:"url" json:"url"`

	// Trust declares how to verify layers from this registry.
	// "strata-core"       — verify against Strata's public signing key
	// "keyfile://<path>"  — verify against a local public key file
	// "keyless"           — verify keyless Sigstore signatures (OIDC)
	Trust string `yaml:"trust" json:"trust"`
}

// InstanceConfig describes the EC2 instance to launch.
type InstanceConfig struct {
	// Type is the EC2 instance type, e.g. "p4d.24xlarge".
	Type string `yaml:"type,omitempty" json:"type,omitempty"`

	// Spot requests a spot instance.
	Spot bool `yaml:"spot,omitempty" json:"spot,omitempty"`

	// SpotFallback is an alternative instance type if the primary is unavailable.
	SpotFallback string `yaml:"spot_fallback,omitempty" json:"spot_fallback,omitempty"`

	// Placement is the EC2 placement strategy, e.g. "cluster".
	Placement string `yaml:"placement,omitempty" json:"placement,omitempty"`

	// Region overrides the default AWS region.
	Region string `yaml:"region,omitempty" json:"region,omitempty"`
}

// StorageMount describes an additional storage volume to mount.
type StorageMount struct {
	// Type is the storage type: "s3", "efs", "ebs", "instance_store".
	Type string `yaml:"type" json:"type"`

	// Mount is the filesystem path to mount at.
	Mount string `yaml:"mount" json:"mount"`

	// Bucket is the S3 bucket name (type=s3 only).
	Bucket string `yaml:"bucket,omitempty" json:"bucket,omitempty"`

	// ID is the EFS filesystem ID (type=efs only).
	ID string `yaml:"id,omitempty" json:"id,omitempty"`

	// ReadOnly mounts the storage read-only.
	ReadOnly bool `yaml:"read_only,omitempty" json:"read_only,omitempty"`
}

// Validate checks that a Profile is well-formed.
// It does not validate against the registry — that is the resolver's job.
func (p Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if err := p.Base.Validate(); err != nil {
		return fmt.Errorf("base: %w", err)
	}
	if len(p.Software) == 0 {
		return fmt.Errorf("software list is required and must not be empty")
	}
	for i, sw := range p.Software {
		if err := sw.Validate(); err != nil {
			return fmt.Errorf("software[%d]: %w", i, err)
		}
	}
	return nil
}
