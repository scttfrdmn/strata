// Package build defines the layer build pipeline and recipe contract.
//
// Layers are pre-built, pre-validated binary artifacts. They are never built
// at profile resolution time — the registry fails loudly if a layer does not
// exist. Layers are produced by the build pipeline defined here.
//
// # Recipe contract
//
// A recipe lives at recipes/<name>/<version>/ and contains two files:
//
//   - build.sh   — install script; must install into $STRATA_PREFIX only
//   - meta.yaml  — declares provides, build_requires, runtime_requires
//
// The build environment contract:
//
//	STRATA_PREFIX    — install here and only here
//	STRATA_NCPUS     — use for parallel builds (nproc)
//	STRATA_ARCH      — target architecture (x86_64, arm64)
//	STRATA_OUT       — output directory for the squashfs (same as STRATA_PREFIX)
//
// build.sh must exit non-zero on any failure. Network access is allowed
// during build but the resulting squashfs must be self-contained.
//
// # Build pipeline stages
//
//  1. Resolve build environment from registry (build_requires)
//  2. Launch clean EC2 instance matching target base
//  3. Mount build environment via Strata overlay
//  4. Execute recipe with STRATA_PREFIX=/strata/out
//  5. Capture /strata/out → squashfs (reproducible options)
//  6. Probe squashfs: what does it actually provide?
//  7. Validate: declared provides ⊆ probed provides
//  8. Generate content manifest (file path → SHA256)
//  9. Sign with cosign → log to Rekor
//  10. Push squashfs + manifest + bundle to S3 registry
//  11. Terminate build instance
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// RecipeMeta is the parsed content of a recipe's meta.yaml file.
// It declares what the layer provides and what it needs at build and runtime.
type RecipeMeta struct {
	// Name is the layer name, e.g. "openmpi".
	Name string `yaml:"name"`

	// Version is the layer version, e.g. "4.1.6".
	Version string `yaml:"version"`

	// Description is optional human-readable context.
	Description string `yaml:"description,omitempty"`

	// Tier classifies where this layer sits in the dependency stack.
	// Valid values: "core", "library", "application".
	//   core:        built with OS system compiler; no build_requires (bootstrap).
	//   library:     built with Strata tools; may be dependency-only (user_selectable:false).
	//   application: end-user software built on top of core/library layers.
	// Required; validated by Validate().
	Tier string `yaml:"tier"`

	// Provides is the list of capabilities this layer makes available.
	// Written into LayerManifest.Provides at build time.
	Provides []spec.Capability `yaml:"provides"`

	// BuildRequires lists capabilities needed during the build but NOT
	// included in the output squashfs. The build environment supplies these.
	BuildRequires []spec.Requirement `yaml:"build_requires,omitempty"`

	// RuntimeRequires lists capabilities required at runtime on the target
	// instance. Written into LayerManifest.Requires. The resolver validates
	// these against BaseCapabilities before mounting.
	RuntimeRequires []spec.Requirement `yaml:"runtime_requires,omitempty"`

	// ABI is the C runtime ABI this layer targets: "linux-gnu-2.34" or "linux-gnu-2.35".
	// A single ABI covers multiple OS versions (AL2023, Rocky 9/10, RHEL 9).
	ABI string `yaml:"abi"`

	// UserSelectable is false for dependency-only layers (ucx, hwloc, pmix, libfabric).
	// Omit from YAML to default to true. ParseRecipe sets this to true when not
	// explicitly set false, because the Go zero value false would hide layers by default.
	UserSelectable *bool `yaml:"user_selectable,omitempty"`

	// InstallLayout is "versioned" (default) or "flat".
	// "flat" layers install directly to $STRATA_PREFIX, not $STRATA_PREFIX/<name>/<version>.
	InstallLayout string `yaml:"install_layout,omitempty"`

	// ModulefileEnv declares extra environment variables to emit in the
	// auto-generated Lmod modulefile beyond the standard auto-detected ones
	// (PATH, LD_LIBRARY_PATH, PKG_CONFIG_PATH, MANPATH, INFOPATH, CPATH).
	ModulefileEnv []ModuleEnvVar `yaml:"modulefile_env,omitempty"`
}

// ModuleEnvVar declares a single environment variable entry for a generated
// Lmod modulefile.
type ModuleEnvVar struct {
	// Var is the environment variable name, e.g. "GCC_HOME", "CC".
	Var string `yaml:"var"`

	// Path is the file path relative to the layer's install prefix.
	// Empty string means the prefix itself (setenv VAR base).
	// A non-empty path is appended: setenv VAR (base .. "/" .. path).
	// Example: path "bin/gcc" → setenv("CC", base .. "/bin/gcc")
	Path string `yaml:"path"`
}

// validTiers is the set of accepted tier values.
var validTiers = map[string]bool{
	"core": true, "library": true, "application": true,
}

// Validate checks that a RecipeMeta is well-formed.
func (m *RecipeMeta) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("recipe meta: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("recipe meta: version is required for %q", m.Name)
	}
	if !validTiers[m.Tier] {
		return fmt.Errorf("recipe meta: %q has unsupported tier %q — supported: core, library, application", m.Name, m.Tier)
	}
	if len(m.Provides) == 0 {
		return fmt.Errorf("recipe meta: %q@%s must declare at least one provides entry", m.Name, m.Version)
	}
	validABIs := map[string]bool{
		"linux-gnu-2.34": true,
		"linux-gnu-2.35": true,
	}
	if !validABIs[m.ABI] {
		return fmt.Errorf("recipe meta: %q has unsupported abi %q — supported: linux-gnu-2.34, linux-gnu-2.35", m.Name, m.ABI)
	}
	validLayouts := map[string]bool{"": true, "versioned": true, "flat": true}
	if !validLayouts[m.InstallLayout] {
		return fmt.Errorf("recipe meta: %q has unsupported install_layout %q", m.Name, m.InstallLayout)
	}
	for i, p := range m.Provides {
		if p.Name == "" {
			return fmt.Errorf("recipe meta: provides[%d] has empty name", i)
		}
	}
	// Core tier invariant: no build_requires (bootstrap build).
	if m.Tier == "core" && len(m.BuildRequires) > 0 {
		return fmt.Errorf("recipe meta: %q is tier core but has build_requires — core layers must use only the OS system compiler", m.Name)
	}
	return nil
}

// Recipe is a fully parsed recipe: the meta.yaml plus the path to build.sh.
type Recipe struct {
	// Meta is the parsed meta.yaml content.
	Meta RecipeMeta

	// BuildScriptPath is the absolute path to build.sh.
	BuildScriptPath string

	// Dir is the recipe directory (parent of build.sh and meta.yaml).
	Dir string
}

// ParseRecipe reads and validates a recipe from dir.
// dir must contain build.sh and meta.yaml.
func ParseRecipe(dir string) (*Recipe, error) {
	metaPath := filepath.Join(dir, "meta.yaml")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("recipe %q: reading meta.yaml: %w", dir, err)
	}

	var meta RecipeMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("recipe %q: parsing meta.yaml: %w", dir, err)
	}

	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("recipe %q: %w", dir, err)
	}

	buildScript := filepath.Join(dir, "build.sh")
	info, err := os.Stat(buildScript)
	if err != nil {
		return nil, fmt.Errorf("recipe %q: build.sh not found: %w", dir, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("recipe %q: build.sh is a directory", dir)
	}

	return &Recipe{
		Meta:            meta,
		BuildScriptPath: buildScript,
		Dir:             dir,
	}, nil
}

// ContentManifest is the per-file content record generated from a built layer.
// Every file in the squashfs is listed with its path and SHA256.
// Used by the resolver for file-level conflict detection.
type ContentManifest struct {
	// LayerID is the layer this manifest describes.
	LayerID string `yaml:"layer_id"`

	// Files maps file path (relative to squashfs root) to SHA256.
	Files map[string]string `yaml:"files"`
}

// ConflictsWith reports whether this manifest conflicts with other.
// Two manifests conflict if any file path appears in both, with different
// SHA256 values. Identical files (same path, same SHA256) are not conflicts.
func (m *ContentManifest) ConflictsWith(other *ContentManifest) []string {
	var conflicts []string
	for path, sha := range m.Files {
		otherSHA, ok := other.Files[path]
		if !ok {
			continue
		}
		if sha != otherSHA {
			conflicts = append(conflicts, path)
		}
	}
	return conflicts
}

// Job describes a single layer build request.
type Job struct {
	// RecipeDir is the path to the recipe directory.
	RecipeDir string

	// Base is the target OS and architecture.
	Base spec.BaseRef

	// RegistryURL is the S3 URL to push the built layer to.
	RegistryURL string

	// DryRun skips the actual EC2 launch and squashfs creation.
	// Used for recipe validation in CI without cloud access.
	DryRun bool

	// EnvResolver resolves and downloads build_requires layers for Stage 3
	// (OverlayFS build environment). When nil, build_requires are not mounted
	// and the layer is marked bootstrap_build=true (Tier 0 only).
	EnvResolver EnvResolver

	// CacheDir is the local directory for caching downloaded .sqfs files.
	// Defaults to os.TempDir()/strata-build-cache when empty.
	CacheDir string

	// KeyRef is the signing key reference (file path or KMS URI) used to
	// sign the layer. Stored as manifest.SignedBy for provenance tracking.
	KeyRef string
}

// Validate checks that a Job is well-formed.
func (j *Job) Validate() error {
	if j.RecipeDir == "" {
		return fmt.Errorf("build job: recipe dir is required")
	}
	if err := j.Base.Validate(); err != nil {
		return fmt.Errorf("build job: base: %w", err)
	}
	if !j.DryRun && j.RegistryURL == "" {
		return fmt.Errorf("build job: registry URL is required for non-dry-run builds")
	}
	if !j.DryRun && !strings.HasPrefix(j.RegistryURL, "s3://") {
		return fmt.Errorf("build job: registry URL must be an S3 URL (s3://...), got %q", j.RegistryURL)
	}
	return nil
}

// SquashfsOptions returns the mksquashfs flags required for reproducible output.
// Same recipe + same build environment = same SHA256.
func SquashfsOptions() []string {
	return []string{
		"-noappend",
		"-no-progress",
		"-comp", "zstd",
		"-mkfs-time", "0",
		"-all-time", "0",
	}
}

// UserSelectableBool returns the effective UserSelectable value.
// Defaults to true if not explicitly set to false in the recipe YAML.
func (m *RecipeMeta) UserSelectableBool() bool {
	if m.UserSelectable == nil {
		return true
	}
	return *m.UserSelectable
}

// ToLayerManifest converts a RecipeMeta to a partial LayerManifest.
// Fields that are populated at build time (SHA256, Source, RekorEntry,
// Bundle, ContentManifest, BuiltAt) are left empty.
func (m *RecipeMeta) ToLayerManifest(arch string) *spec.LayerManifest {
	id := m.Name + "-" + m.Version + "-" + m.ABI + "-" + arch
	return &spec.LayerManifest{
		ID:             id,
		Name:           m.Name,
		Version:        m.Version,
		Arch:           arch,
		ABI:            m.ABI,
		UserSelectable: m.UserSelectableBool(),
		InstallLayout:  m.InstallLayout,
		Provides:       m.Provides,
		Requires:       m.RuntimeRequires,
	}
}
