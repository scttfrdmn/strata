package strata_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	pkgstrata "github.com/scttfrdmn/strata/pkg/strata"
	"github.com/scttfrdmn/strata/spec"
)

// malformedLockfile carries a present-but-invalid layer digest, which
// LockFile.Validate rejects. A layer with an empty SHA256 would be a valid
// unfrozen lockfile, so the digest must be non-empty and malformed.
func malformedLockfile() *spec.LockFile {
	return &spec.LockFile{
		ProfileName: "evil",
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "x-1.0-x86_64", SHA256: "bbbbbb"}, MountOrder: 1},
		},
	}
}

// TestLockfileUserData_RejectsInvalid: the agent treats user-data as its
// highest-priority lockfile source, so LockfileUserData must refuse a
// structurally invalid lockfile rather than serialise it (#65/#96, review
// follow-up).
func TestLockfileUserData_RejectsInvalid(t *testing.T) {
	if _, err := pkgstrata.LockfileUserData(malformedLockfile()); err == nil {
		t.Fatal("LockfileUserData serialised a lockfile with a malformed digest")
	}
}

// TestUploadLockfile_RejectsInvalid: the library upload path must refuse a
// malformed lockfile before persisting it, and before the S3-client check, so
// the input is judged on its own terms. Reached through the registry-injection
// seam (no live S3).
func TestUploadLockfile_RejectsInvalid(t *testing.T) {
	c := pkgstrata.NewClientFromRegistry(registry.NewMemoryStore(), "")
	if _, err := c.UploadLockfile(context.Background(), malformedLockfile()); err == nil {
		t.Fatal("UploadLockfile persisted a lockfile with a malformed digest")
	}
}
