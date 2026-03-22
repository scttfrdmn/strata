// Package scan detects installed software on the current system and matches
// it against the Strata layer registry.
package scan

import "github.com/scttfrdmn/strata/spec"

// DetectSource identifies how a package was found.
type DetectSource string

// DetectSource constants identify where a package was found.
const (
	SourceLmod       DetectSource = "lmod"
	SourceConda      DetectSource = "conda"
	SourcePip        DetectSource = "pip"
	SourceFilesystem DetectSource = "filesystem"
)

// DetectedPackage describes a software installation found on the current system.
type DetectedPackage struct {
	Name    string
	Version string
	Prefix  string // install root (empty for pip packages)
	Source  DetectSource

	ModulefilePath string   // lmod only
	ModuleDeps     []string // lmod prereqs
	Channel        string   // conda only
	BuildHash      string   // conda only
}

// MatchStatus describes how a DetectedPackage matched the registry.
type MatchStatus string

// MatchStatus constants indicate how well a detected package matched the registry.
const (
	StatusMatched   MatchStatus = "matched"    // exact name+version in registry
	StatusNearMatch MatchStatus = "near_match" // name found, different version
	StatusUnmatched MatchStatus = "unmatched"  // name not in registry
)

// MatchResult pairs a detected package with its registry match status.
type MatchResult struct {
	Package     DetectedPackage
	Status      MatchStatus
	Manifest    *spec.LayerManifest // non-nil for Matched/NearMatch
	NearVersion string              // nearest version (NearMatch only)
}
