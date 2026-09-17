// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package spec

import "sort"

// BuildEnvironment records the pinned OS toolchain a layer was built against
// (#234, properties B2/B3). Every EC2 build installs a compiler toolchain and
// -devel headers from the AL2023 repos before compiling; recording them here
// names every build input (B2) rather than leaving it to the moving repos, and
// pinning the dnf releasever to the fixed AMI's own snapshot makes the toolchain
// deterministic (B3, up to the AL2023 versioned repo and the fixed AMI).
//
// It extends the manifest's BootstrapCompiler — which named only gcc — to the
// whole toolchain. A layer built purely from Strata layers records those in
// BuiltWith instead and leaves this empty. A nil or empty value means the build
// environment was not recorded, which does not discharge B2 (see Recorded).
type BuildEnvironment struct {
	// AMIID is the base AMI the build ran on — a fixed image, so it is itself a
	// pinned input.
	AMIID string `yaml:"ami_id,omitempty" json:"ami_id,omitempty"`

	// Releasever is the AL2023 dnf releasever snapshot the toolchain was resolved
	// from. Pinned to the AMI's own version so the same recipe resolves the same
	// packages over time rather than whatever the repos currently serve.
	Releasever string `yaml:"releasever,omitempty" json:"releasever,omitempty"`

	// Packages is the exact NVR of every OS package installed on the build
	// instance (e.g. "gcc-11.4.1-2.amzn2023.0.1.x86_64"), sorted and de-duplicated
	// so the recorded set is order-stable regardless of how rpm -qa emitted it.
	Packages []string `yaml:"packages,omitempty" json:"packages,omitempty"`
}

// NewBuildEnvironment builds a BuildEnvironment with packages sorted and
// de-duplicated. A build's provenance must not depend on enumeration order —
// the same determinism EnvironmentID demands elsewhere — so two builds that
// installed the same packages in a different order record an identical set.
func NewBuildEnvironment(amiID, releasever string, packages []string) *BuildEnvironment {
	return &BuildEnvironment{
		AMIID:      amiID,
		Releasever: releasever,
		Packages:   sortDedupStrings(packages),
	}
}

// Recorded reports whether this build environment actually names its inputs: an
// AMI and at least one package. It is the predicate B2 turns on — an unset or
// half-filled BuildEnvironment does not discharge "every build input is named".
func (e *BuildEnvironment) Recorded() bool {
	return e != nil && e.AMIID != "" && len(e.Packages) > 0
}

// sortDedupStrings returns the input sorted with empties and duplicates removed,
// or nil for an empty result (so it serialises as an absent field, not []).
func sortDedupStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
