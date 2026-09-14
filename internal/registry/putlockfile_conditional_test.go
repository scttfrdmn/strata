package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// #124: PutLockfile built the storage key by concatenation and wrote
// unconditionally, so an unfrozen lockfile (empty EnvironmentID) landed at
// locks/.yaml and a colliding EnvironmentID silently overwrote (last writer
// wins). Both clients now refuse an empty id, and write conditionally — local via
// O_EXCL, S3 via IfNoneMatch. The S3 collision path is held by reading: this
// package's mockS3 ignores IfNoneMatch, so only the empty-id refusal is executed
// for S3; the local O_EXCL guard exercises the collision mechanism directly.
//
// Replaces lockfile_key_test.go, which pinned the defect and instructed its own
// deletion once #124 was fixed.

// putFrozenFixture: no layers + a non-empty base digest ⇒ IsFrozen ⇒ non-empty
// EnvironmentID, so PutLockfile builds a real key.
func putFrozenFixture(profile string) *spec.LockFile {
	return &spec.LockFile{ProfileName: profile, Base: spec.ResolvedBase{AMISHA256: strings.Repeat("a", 64)}}
}

func TestPutLockfile_RefusesUnfrozen(t *testing.T) {
	ctx := context.Background()
	unfrozen := &spec.LockFile{ProfileName: "unfrozen"} // no AMISHA256 ⇒ EnvironmentID ""
	if unfrozen.EnvironmentID() != "" {
		t.Fatalf("premise gone: unfrozen lockfile has id %q, want empty", unfrozen.EnvironmentID())
	}

	t.Run("local", func(t *testing.T) {
		dir := t.TempDir()
		c, err := NewLocalClient("file://" + dir)
		if err != nil {
			t.Fatalf("NewLocalClient: %v", err)
		}
		if _, err := c.PutLockfile(ctx, unfrozen); err == nil {
			t.Fatal("stored an unfrozen lockfile; expected a refusal (#124)")
		} else if !strings.Contains(err.Error(), "no EnvironmentID") {
			t.Errorf("refusal must name the missing id, got: %v", err)
		}
		// NewLocalClient pre-creates locks/; the refusal must leave it empty
		// (no locks/.yaml, no anything).
		if entries, err := os.ReadDir(filepath.Join(dir, "locks")); err != nil {
			t.Fatalf("reading locks/: %v", err)
		} else if len(entries) != 0 {
			t.Errorf("a refused put wrote %d file(s) into locks/, want 0", len(entries))
		}
	})

	t.Run("s3", func(t *testing.T) {
		mock := newMockS3()
		c := newS3ClientWithAPI("bucket", mock)
		if _, err := c.PutLockfile(ctx, unfrozen); err == nil {
			t.Fatal("stored an unfrozen lockfile; expected a refusal (#124)")
		} else if !strings.Contains(err.Error(), "no EnvironmentID") {
			t.Errorf("refusal must name the missing id, got: %v", err)
		}
		if len(mock.objects) != 0 {
			t.Errorf("a refused put wrote %d objects, want 0", len(mock.objects))
		}
	})
}

// TestPutLockfile_LocalRefusesCollision exercises the collision guard directly:
// a second write to an existing EnvironmentID key must be a loud error, not a
// silent overwrite. Putting the same frozen lockfile twice reaches the same key.
func TestPutLockfile_LocalRefusesCollision(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c, err := NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatalf("NewLocalClient: %v", err)
	}
	lf := putFrozenFixture("env-a")
	if lf.EnvironmentID() == "" {
		t.Fatal("premise gone: fixture lockfile is not frozen")
	}

	if _, err := c.PutLockfile(ctx, lf); err != nil {
		t.Fatalf("first PutLockfile: %v", err)
	}
	if _, err := c.PutLockfile(ctx, lf); err == nil {
		t.Fatal("second PutLockfile at the same key succeeded; expected a collision refusal (#124)")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("collision refusal must say so, got: %v", err)
	}
}
