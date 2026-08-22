package registry_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestPutLockfile_UnfrozenLockfilesShareOneKey executes the claim that #124's
// register row in PROPERTIES.md previously held by reading.
//
// `EnvironmentID()` returns "" for an unfrozen lockfile, saying it has no
// identity. Both registry clients build the storage key by concatenation —
// `internal/registry/localclient.go:374` and `s3client.go:617` write
// "locks/" + lockfile.EnvironmentID() + ".yaml" — so every unfrozen lockfile
// lands on the single key `locks/.yaml`, and the put is unconditional. The
// undefined case is being used as a value.
//
// This test asserts that the defect **still reproduces**, so it fails when #124
// is fixed. That failure is the instruction to delete this file, not to update
// it: the correct behaviour is a refusal before any key is built, and there is
// then nothing here left to assert. Do not "fix" it by asserting the new key —
// there is no key.
//
// Only the local client is executed. `s3client.go` is uncovered (rule 6 in
// PROPERTIES.md §7), so the S3 half of the same claim remains held by reading;
// the two clients build the key with the same expression on adjacent lines.
func TestPutLockfile_UnfrozenLockfilesShareOneKey(t *testing.T) {
	dir := t.TempDir()
	c, err := registry.NewLocalClient("file://" + dir)
	if err != nil {
		t.Fatalf("NewLocalClient: %v", err)
	}
	ctx := context.Background()

	// Two lockfiles that differ, and that a reader would call different
	// environments. Neither is frozen: no AMISHA256.
	alpha := &spec.LockFile{ProfileName: "alpha"}
	beta := &spec.LockFile{ProfileName: "beta"}

	// The premise, asserted rather than assumed: if these ever become frozen the
	// rest of this test measures something else entirely and would still pass.
	for _, lf := range []*spec.LockFile{alpha, beta} {
		if lf.IsFrozen() {
			t.Fatalf("premise gone: %q is frozen, so this test no longer reaches the unfrozen path", lf.ProfileName)
		}
		if id := lf.EnvironmentID(); id != "" {
			t.Fatalf("premise gone: %q has EnvironmentID %q, want empty", lf.ProfileName, id)
		}
	}

	uriAlpha, err := c.PutLockfile(ctx, alpha)
	if err != nil {
		t.Fatalf("PutLockfile(alpha): %v — if this is now a refusal, #124 is fixed and this file should be deleted", err)
	}
	uriBeta, err := c.PutLockfile(ctx, beta)
	if err != nil {
		t.Fatalf("PutLockfile(beta): %v — if this is now a refusal, #124 is fixed and this file should be deleted", err)
	}
	if uriAlpha != uriBeta {
		t.Errorf("the two puts returned different URIs (%q, %q); #124's collision no longer "+
			"reproduces and this file should be deleted", uriAlpha, uriBeta)
	}

	// The key itself, named as a literal: a dotfile, which is the shape of the
	// defect and not merely a shared name.
	entries, err := os.ReadDir(filepath.Join(dir, "locks"))
	if err != nil {
		t.Fatalf("ReadDir(locks): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("locks/ holds %d files after two puts, want 1 — the collision is what #124 is about", len(entries))
	}
	if got := entries[0].Name(); got != ".yaml" {
		t.Errorf("the stored key is %q, want %q — an empty identity concatenated into a filename", got, ".yaml")
	}

	// And the consequence, which is why the collision is destructive rather than
	// ambiguous: the second writer's lockfile is the one a reader gets back.
	recs, err := c.ListLockfiles(ctx)
	if err != nil {
		t.Fatalf("ListLockfiles: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("ListLockfiles returned %d records, want 1", len(recs))
	}
	if got := recs[0].LockFile.ProfileName; got != "beta" {
		t.Errorf("the stored lockfile is %q, want %q — last writer wins, so which environment is "+
			"served depends on publish order and not on any identity", got, "beta")
	}
}
