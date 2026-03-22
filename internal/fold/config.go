// Package fold merges assembled Strata environments into portable artifacts.
//
// Two modes:
//   - MergeToLayer: merge N squashfs layers into a single squashfs layer (Linux only).
//   - EjectToDir: materialize the merged overlay into a plain directory tree (all platforms).
package fold

import (
	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/spec"
)

// MergeConfig controls MergeToLayer behavior.
type MergeConfig struct {
	// Name is the name of the merged layer (required).
	Name string

	// Version is the version of the merged layer (required).
	Version string

	// ABI is the C runtime ABI identifier (e.g. "linux-gnu-2.34").
	// Defaults to the ABI of the first layer in the lockfile.
	ABI string

	// Arch is the target architecture (e.g. "x86_64", "arm64").
	// Defaults to the Arch of the first layer in the lockfile.
	Arch string

	// Provides declares capabilities the merged layer exports.
	Provides []spec.Capability

	// Requires declares capability requirements of the merged layer.
	Requires []spec.Requirement

	// KeyRef is the cosign key reference (file path or KMS URI).
	// Empty means skip signing.
	KeyRef string

	// NoSign disables signing even if KeyRef is set.
	NoSign bool

	// Registry is the destination registry client. Required unless DryRun.
	Registry build.PushRegistry

	// DryRun prints the plan without executing.
	DryRun bool
}

// EjectConfig controls EjectToDir behavior.
type EjectConfig struct {
	// OutputDir is the destination directory. It must not already exist
	// (or must be empty). Required.
	OutputDir string

	// CacheDir is the directory where layer .sqfs files are cached.
	// Defaults to the system cache dir if empty.
	CacheDir string
}
