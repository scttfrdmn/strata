package spec_test

import (
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func validDigest() string { return strings.Repeat("a", 64) }

func layer(id, sha256 string, mountOrder int) spec.ResolvedLayer {
	return spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{ID: id, SHA256: sha256},
		MountOrder:    mountOrder,
	}
}

// TestLockFile_Validate_AcceptsWellFormed covers the two states a valid lockfile
// can be in: frozen (digests present and well-formed) and unfrozen (digests
// empty). Validate is a well-formedness check, not a frozen check.
func TestLockFile_Validate_AcceptsWellFormed(t *testing.T) {
	frozen := &spec.LockFile{
		Base:   spec.ResolvedBase{AMISHA256: validDigest()},
		Layers: []spec.ResolvedLayer{layer("python-3.13.2-linux-gnu-2.34-x86_64", validDigest(), 1)},
	}
	if err := frozen.Validate(); err != nil {
		t.Errorf("well-formed frozen lockfile rejected: %v", err)
	}

	unfrozen := &spec.LockFile{
		Layers: []spec.ResolvedLayer{layer("python-3.13.2-linux-gnu-2.34-x86_64", "", 1)},
	}
	if err := unfrozen.Validate(); err != nil {
		t.Errorf("unfrozen lockfile (empty digests) rejected — that is a valid intermediate state: %v", err)
	}
}

// TestLockFile_Validate_RejectsMalformed is #96: a non-empty digest that is not
// 64 lowercase hex must be rejected, on both a layer and the base — the values
// the repo's own fixtures used while passing IsFrozen.
func TestLockFile_Validate_RejectsMalformed(t *testing.T) {
	cases := map[string]*spec.LockFile{
		"short layer digest (bbbbbb)": {
			Base:   spec.ResolvedBase{AMISHA256: validDigest()},
			Layers: []spec.ResolvedLayer{layer("python-3.13.2-linux-gnu-2.34-x86_64", "bbbbbb", 1)},
		},
		"non-hex base ami_sha256": {
			Base:   spec.ResolvedBase{AMISHA256: "sha256-ami-test123456789"},
			Layers: []spec.ResolvedLayer{layer("python-3.13.2-linux-gnu-2.34-x86_64", validDigest(), 1)},
		},
		"uppercase digest": {
			Base:   spec.ResolvedBase{AMISHA256: validDigest()},
			Layers: []spec.ResolvedLayer{layer("python-3.13.2-linux-gnu-2.34-x86_64", strings.ToUpper(validDigest()), 1)},
		},
		"path-traversal layer id": {
			Base:   spec.ResolvedBase{AMISHA256: validDigest()},
			Layers: []spec.ResolvedLayer{layer("../../etc/cron.d/evil", validDigest(), 1)},
		},
		"duplicate mount order (#95 hash-tie)": {
			Base: spec.ResolvedBase{AMISHA256: validDigest()},
			Layers: []spec.ResolvedLayer{
				layer("alpha-1.0-linux-gnu-2.34-x86_64", validDigest(), 1),
				layer("bravo-1.0-linux-gnu-2.34-x86_64", validDigest(), 1),
			},
		},
	}
	for name, lf := range cases {
		t.Run(name, func(t *testing.T) {
			if err := lf.Validate(); err == nil {
				t.Errorf("Validate accepted a malformed lockfile (%s)", name)
			}
		})
	}
}
