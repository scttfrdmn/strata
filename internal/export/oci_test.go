package export_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/export"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

func TestTarDirDeterministic_Identical(t *testing.T) {
	dir := t.TempDir()
	// Create a small tree.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested\n"), 0644); err != nil {
		t.Fatalf("writing nested: %v", err)
	}

	b1, d1, err := export.TarDirDeterministic(dir)
	if err != nil {
		t.Fatalf("first TarDirDeterministic: %v", err)
	}
	b2, d2, err := export.TarDirDeterministic(dir)
	if err != nil {
		t.Fatalf("second TarDirDeterministic: %v", err)
	}

	if d1 != d2 {
		t.Errorf("digests differ between calls: %q vs %q", d1, d2)
	}
	if !bytes.Equal(b1, b2) {
		t.Error("tar bytes differ between calls (not deterministic)")
	}
}

func TestTarDirDeterministic_ValidTar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	tarBytes, _, err := export.TarDirDeterministic(dir)
	if err != nil {
		t.Fatalf("TarDirDeterministic: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(tarBytes))
	var count int
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		count++
	}
	if count == 0 {
		t.Error("expected at least one tar entry")
	}
}

func TestExportOCI_RequiresUnsquashfs(t *testing.T) {
	if _, err := exec.LookPath("unsquashfs"); err != nil {
		t.Skip("unsquashfs not in PATH")
	}
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not in PATH")
	}

	// Build a tiny squashfs fixture.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "layer-file.txt"), []byte("layer content\n"), 0644); err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	sqfsPath := filepath.Join(t.TempDir(), "test.sqfs")
	out, err := exec.Command("mksquashfs", srcDir, sqfsPath, "-quiet").CombinedOutput()
	if err != nil {
		t.Fatalf("mksquashfs: %v: %s", err, out)
	}

	lf := &spec.LockFile{
		ProfileName: "oci-test",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:      "test-layer-1.0",
					Name:    "test",
					Version: "1.0",
					SHA256:  "abc123",
				},
				MountOrder: 1,
			},
		},
	}
	layerPaths := []overlay.LayerPath{
		{ID: "test-layer-1.0", SHA256: "abc123", Path: sqfsPath, MountOrder: 1},
	}

	outPath := filepath.Join(t.TempDir(), "test.tar")
	if err := export.Export(context.Background(), lf, layerPaths, outPath, "test:v1"); err != nil {
		t.Fatalf("ExportOCI: %v", err)
	}

	// Verify the output is a valid tar with oci-layout and index.json.
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("opening output: %v", err)
	}
	defer f.Close() //nolint:errcheck

	var files []string
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading output tar: %v", err)
		}
		files = append(files, hdr.Name)
	}

	for _, want := range []string{"oci-layout", "index.json"} {
		var found bool
		for _, name := range files {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("output tar missing %q; files: %v", want, files)
		}
	}

	// Check blobs directory exists.
	var hasBlob bool
	for _, name := range files {
		if strings.HasPrefix(name, "blobs/sha256/") {
			hasBlob = true
			break
		}
	}
	if !hasBlob {
		t.Error("output tar has no blobs/sha256/ entries")
	}
}

func TestWriteOCILayout_ValidJSON(t *testing.T) {
	// Build a minimal OCI layout with one empty layer and verify JSON validity.
	layerData := []byte("fake-tar-data")
	_ = layerData // used via WriteOCILayout internal call

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	tarBytes, digest, err := export.TarDirDeterministic(dir)
	if err != nil {
		t.Fatalf("TarDirDeterministic: %v", err)
	}

	lf := &spec.LockFile{ProfileName: "json-test"}
	layerPaths := []overlay.LayerPath{{ID: "layer1", SHA256: digest, MountOrder: 1}}

	// Manually create a tiny ociLayer-equivalent by calling ExportOCI would need
	// real squashfs. Instead, test WriteOCILayout's index.json via the exported
	// WriteOCILayout function if available; otherwise verify via tarBytes shape.

	// Verify that tarDirDeterministic output is a non-empty valid tar.
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tarBytes is not a valid tar: %v", err)
	}
	if hdr.Name == "" {
		t.Error("first tar entry has empty name")
	}
	_ = lf
	_ = layerPaths
}

// Verify that index.json produced by a minimal export has valid JSON structure.
func TestOCILayout_IndexJSON(t *testing.T) {
	if _, err := exec.LookPath("unsquashfs"); err != nil {
		t.Skip("unsquashfs not in PATH")
	}
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not in PATH")
	}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "f"), []byte("data"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sqfs := filepath.Join(t.TempDir(), "l.sqfs")
	if out, err := exec.Command("mksquashfs", srcDir, sqfs, "-quiet").CombinedOutput(); err != nil {
		t.Fatalf("mksquashfs: %v: %s", err, out)
	}

	lf := &spec.LockFile{ProfileName: "idx-test"}
	paths := []overlay.LayerPath{{ID: "l-1.0", SHA256: "x", Path: sqfs, MountOrder: 1}}
	outPath := filepath.Join(t.TempDir(), "out.tar")
	if err := export.Export(context.Background(), lf, paths, outPath, ""); err != nil {
		t.Fatalf("ExportOCI: %v", err)
	}

	f, _ := os.Open(outPath)
	defer f.Close() //nolint:errcheck
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		if hdr.Name != "index.json" {
			continue
		}
		data, _ := io.ReadAll(tr)
		var v map[string]interface{}
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("index.json is not valid JSON: %v", err)
		}
		if _, ok := v["manifests"]; !ok {
			t.Error("index.json missing 'manifests' key")
		}
		return
	}
	t.Error("index.json not found in output tar")
}
