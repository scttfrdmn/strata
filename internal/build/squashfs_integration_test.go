//go:build integration

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateSquashfs_Integration(t *testing.T) {
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not on PATH")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "test.sqfs")

	if err := CreateSquashfs(context.Background(), dir, outPath); err != nil {
		t.Fatalf("CreateSquashfs: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-zero sqfs size")
	}
}
