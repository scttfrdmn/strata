package spec

import "fmt"

// PackageManager identifies the package manager for a set of packages.
type PackageManager string

const (
	// PackageManagerPip installs Python packages via pip.
	PackageManagerPip PackageManager = "pip"
	// PackageManagerConda installs packages via conda/mamba.
	PackageManagerConda PackageManager = "conda"
	// PackageManagerCRAN installs R packages from CRAN.
	PackageManagerCRAN PackageManager = "cran"
)

// PackageEntry is a single declared package with an optional version constraint.
// The Version field is a version prefix or constraint (e.g. "1.26" matches
// "1.26.0", "1.26.1", etc.), not necessarily a pinned exact version.
// Pinned exact versions appear in ResolvedPackageEntry after strata freeze.
type PackageEntry struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// PackageSpec declares one package manager's entries in a profile.
// Env is only valid for conda — it specifies the conda environment name to
// install into (defaults to "base" if empty and manager is conda).
type PackageSpec struct {
	Manager  PackageManager `yaml:"manager" json:"manager"`
	Packages []PackageEntry `yaml:"packages" json:"packages"`
	Env      string         `yaml:"env,omitempty" json:"env,omitempty"`
}

// ResolvedPackageEntry is an exact pinned version produced by strata freeze.
// SHA256 is the content hash of the downloaded wheel or archive; it is
// populated when available but may be empty for some package managers.
type ResolvedPackageEntry struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	SHA256  string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// ResolvedPackageSet is the frozen representation of one PackageSpec in the
// lockfile. All versions are exactly pinned.
type ResolvedPackageSet struct {
	Manager  PackageManager         `yaml:"manager" json:"manager"`
	Env      string                 `yaml:"env,omitempty" json:"env,omitempty"`
	Packages []ResolvedPackageEntry `yaml:"packages" json:"packages"`
}

// MutableLayerSpec declares a persistent EBS upper directory for the Path B
// interactive workflow. The agent attaches (or creates) an EBS volume and
// mounts it as the OverlayFS upper, allowing persistent per-session installs
// that can later be frozen into a signed squashfs layer with strata freeze-layer.
type MutableLayerSpec struct {
	// Name is the layer name that will be used when freeze-layer produces
	// the signed squashfs (e.g. "torch-ml").
	Name string `yaml:"name" json:"name"`

	// Version is the layer version assigned at freeze-layer time.
	// It is recorded in the lockfile so the agent can tag the EBS volume
	// for identification, but is not used for resolution until after freeze.
	Version string `yaml:"version" json:"version"`

	// SizeGB is the EBS volume size in GiB. Defaults to 20 if zero.
	SizeGB int `yaml:"size_gb,omitempty" json:"size_gb,omitempty"`

	// VolumeType is the EBS volume type. Defaults to "gp3" if empty.
	VolumeType string `yaml:"volume_type,omitempty" json:"volume_type,omitempty"`

	// ABI is the C runtime ABI for the resulting layer. If empty, it is
	// inferred from the base capabilities at resolution time.
	ABI string `yaml:"abi,omitempty" json:"abi,omitempty"`
}

// Validate checks that a MutableLayerSpec is well-formed.
func (m MutableLayerSpec) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("mutable_layer: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("mutable_layer: version is required")
	}
	return nil
}

// ValidatePackageSpec checks that a PackageSpec is well-formed.
func ValidatePackageSpec(p PackageSpec) error {
	switch p.Manager {
	case PackageManagerPip, PackageManagerConda, PackageManagerCRAN:
		// valid
	default:
		return fmt.Errorf("unsupported package manager %q — supported: pip, conda, cran", p.Manager)
	}
	if len(p.Packages) == 0 {
		return fmt.Errorf("packages list must not be empty for manager %q", p.Manager)
	}
	for i, pkg := range p.Packages {
		if pkg.Name == "" {
			return fmt.Errorf("packages[%d]: name is required", i)
		}
	}
	if p.Env != "" && p.Manager != PackageManagerConda {
		return fmt.Errorf("env field is only valid for manager %q", PackageManagerConda)
	}
	return nil
}
