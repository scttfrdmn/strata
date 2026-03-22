package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreezeLayerDryRun(t *testing.T) {
	// Create a non-empty upper directory.
	upper := t.TempDir()
	if err := os.WriteFile(filepath.Join(upper, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// dry-run should succeed without a registry.
	err := runFreezeLayer(
		t.Context(),
		upper,            // upperDir
		"torch-ml",       // name
		"0.1.0",          // version
		"linux-gnu-2.34", // abi
		"x86_64",         // arch
		"",               // reg (not needed for dry-run)
		"",               // key
		"torch=2.2.0",    // provides
		"glibc@>=2.34",   // requires
		true,             // dryRun
	)
	if err != nil {
		t.Errorf("dry-run should not error, got: %v", err)
	}
}

func TestFreezeLayerMissingUpper(t *testing.T) {
	err := runFreezeLayer(
		t.Context(),
		"/nonexistent/upper/dir",
		"torch-ml",
		"0.1.0",
		"linux-gnu-2.34",
		"x86_64",
		"",
		"",
		"",
		"",
		true, // dry-run to avoid needing registry
	)
	if err == nil {
		t.Error("expected error for nonexistent upper directory")
	}
	if !strings.Contains(err.Error(), "upper") {
		t.Errorf("error should mention --upper, got: %v", err)
	}
}

func TestFreezeLayerEmptyUpper(t *testing.T) {
	upper := t.TempDir() // empty
	err := runFreezeLayer(
		t.Context(),
		upper,
		"torch-ml",
		"0.1.0",
		"linux-gnu-2.34",
		"x86_64",
		"",
		"",
		"",
		"",
		true,
	)
	if err == nil {
		t.Error("expected error for empty upper directory")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestFreezeLayerMissingRegistry(t *testing.T) {
	upper := t.TempDir()
	if err := os.WriteFile(filepath.Join(upper, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-dry-run without registry should fail.
	err := runFreezeLayer(
		t.Context(),
		upper,
		"torch-ml",
		"0.1.0",
		"linux-gnu-2.34",
		"x86_64",
		"", // no registry
		"",
		"",
		"",
		false, // not dry-run
	)
	if err == nil {
		t.Error("expected error when registry is missing and not dry-run")
	}
}

func TestFreezeLayerParseCapabilities(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{"", 0, false},
		{"torch=2.2.0", 1, false},
		{"torch=2.2.0,python=3.12.13", 2, false},
		{"invalid-no-equals", 0, true},
		{"=noname", 0, true},
		{"name=", 0, true},
	}
	for _, tt := range tests {
		caps, err := parseCapabilities(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseCapabilities(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && len(caps) != tt.wantLen {
			t.Errorf("parseCapabilities(%q): got %d caps, want %d", tt.input, len(caps), tt.wantLen)
		}
	}
}

func TestFreezeLayerParseRequirements(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
	}{
		{"", 0},
		{"glibc@>=2.34", 1},
		{"glibc@>=2.34,python@>=3.12", 2},
		{"glibc", 1}, // plain name, no version
	}
	for _, tt := range tests {
		reqs, err := parseRequirements(tt.input)
		if err != nil {
			t.Errorf("parseRequirements(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if len(reqs) != tt.wantLen {
			t.Errorf("parseRequirements(%q): got %d reqs, want %d", tt.input, len(reqs), tt.wantLen)
		}
	}
}

// TestFreezeLayerLocalRegistry performs a full round-trip: build a populated
// directory, freeze-layer into a local registry, verify manifest is present.
// Skipped when mksquashfs is not available (CI environments without it).
func TestFreezeLayerLocalRegistry(t *testing.T) {
	// Check mksquashfs availability.
	if _, err := lookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not on PATH — skipping round-trip test")
	}
	// Check cosign availability.
	if _, err := lookPath("cosign"); err != nil {
		t.Skip("cosign not on PATH — skipping round-trip test")
	}

	upper := t.TempDir()
	if err := os.WriteFile(filepath.Join(upper, "hello.sh"), []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Round-trip requires a real cosign key; skip in unit test context.
	t.Skip("round-trip requires a real cosign key — covered by integration tests")

	regDir := t.TempDir()
	regURL := "file://" + regDir
	var keyRef string

	err := runFreezeLayer(
		t.Context(),
		upper,
		"test-layer",
		"0.1.0",
		"linux-gnu-2.34",
		"x86_64",
		regURL,
		keyRef,
		"test=0.1.0",
		"glibc@>=2.34",
		false,
	)
	if err != nil {
		t.Fatalf("freeze-layer round-trip: %v", err)
	}

	// Verify manifest.yaml was written.
	manifestPath := filepath.Join(regDir, "layers", "linux-gnu-2.34", "x86_64", "test-layer", "0.1.0", "manifest.yaml")
	if _, statErr := os.Stat(manifestPath); statErr != nil {
		t.Errorf("expected manifest.yaml at %s: %v", manifestPath, statErr)
	}
}

// lookPath is a thin wrapper for tests that avoids importing os/exec at package level.
func lookPath(name string) (string, error) {
	// Use a bytes buffer to hold the output of 'which' for portability.
	var buf bytes.Buffer
	_ = buf
	// Check via os.LookupEnv or simple file existence in common paths.
	for _, dir := range strings.Split(os.Getenv("PATH"), ":") {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}
