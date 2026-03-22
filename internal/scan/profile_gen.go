package scan

import (
	"time"

	"github.com/scttfrdmn/strata/spec"
)

// ProfileFromMatches builds a Profile from StatusMatched results only.
func ProfileFromMatches(results []MatchResult, name, osName, arch string) *spec.Profile {
	p := &spec.Profile{
		Name: name,
		Base: spec.BaseRef{
			OS:   osName,
			Arch: arch,
		},
	}

	for _, r := range results {
		if r.Status != StatusMatched {
			continue
		}
		p.Software = append(p.Software, spec.SoftwareRef{
			Name:    r.Package.Name,
			Version: r.Package.Version,
		})
	}

	return p
}

// LockFileFromMatches builds a LockFile from StatusMatched results only.
// MountOrder is detection order. ResolvedAt is time.Now(). RekorEntry is empty.
func LockFileFromMatches(results []MatchResult, name, osName, arch, abi string) *spec.LockFile {
	lf := &spec.LockFile{
		ProfileName: name,
		ResolvedAt:  time.Now().UTC(),
		Base: spec.ResolvedBase{
			DeclaredOS: osName,
			Capabilities: spec.BaseCapabilities{
				OS:   osName,
				Arch: arch,
				ABI:  abi,
			},
		},
	}

	for i, r := range results {
		if r.Status != StatusMatched || r.Manifest == nil {
			continue
		}
		lf.Layers = append(lf.Layers, spec.ResolvedLayer{
			LayerManifest: *r.Manifest,
			MountOrder:    i,
		})
	}

	return lf
}
