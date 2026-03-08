package build

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateSquashfs_NotFound(t *testing.T) {
	// Only run this test when mksquashfs is absent; skip when it is present.
	if _, err := exec.LookPath("mksquashfs"); err == nil {
		t.Skip("mksquashfs is present on PATH — skipping not-found test")
	}

	dir := t.TempDir()
	outPath := filepath.Join(dir, "test.sqfs")

	err := CreateSquashfs(context.Background(), dir, outPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFound *ErrMksquashfsNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("expected *ErrMksquashfsNotFound, got %T: %v", err, err)
	}
}
