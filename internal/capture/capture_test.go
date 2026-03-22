package capture

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestCapture_DryRunRequiresName(t *testing.T) {
	_, err := Capture(context.Background(), Config{
		DryRun: true,
	})
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrNotSupported) {
			t.Errorf("non-linux: expected ErrNotSupported, got %v", err)
		}
		return
	}
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestCapture_DryRunRequiresVersion(t *testing.T) {
	_, err := Capture(context.Background(), Config{
		Name:   "mylib",
		DryRun: true,
	})
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrNotSupported) {
			t.Errorf("non-linux: expected ErrNotSupported, got %v", err)
		}
		return
	}
	if err == nil {
		t.Error("expected error for missing version, got nil")
	}
}

func TestCapture_DryRun(t *testing.T) {
	result, err := Capture(context.Background(), Config{
		Name:    "mylib",
		Version: "1.0.0",
		Prefix:  "/opt/mylib/1.0.0",
		ABI:     "linux-gnu-2.34",
		Arch:    "x86_64",
		DryRun:  true,
	})
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrNotSupported) {
			t.Errorf("non-linux: expected ErrNotSupported, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("dry-run capture failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Manifest == nil {
		t.Fatal("manifest is nil")
	}
	if result.Manifest.Name != "mylib" {
		t.Errorf("Name = %q, want mylib", result.Manifest.Name)
	}
	if result.Manifest.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", result.Manifest.Version)
	}
	if result.Manifest.ABI != "linux-gnu-2.34" {
		t.Errorf("ABI = %q, want linux-gnu-2.34", result.Manifest.ABI)
	}
	if result.Manifest.Arch != "x86_64" {
		t.Errorf("Arch = %q, want x86_64", result.Manifest.Arch)
	}
}

func TestCapture_RequiresRegistry(t *testing.T) {
	_, err := Capture(context.Background(), Config{
		Name:    "mylib",
		Version: "1.0.0",
		Prefix:  "/opt/mylib/1.0.0",
		DryRun:  false,
		// Registry is nil
	})
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrNotSupported) {
			t.Errorf("non-linux: expected ErrNotSupported, got %v", err)
		}
		return
	}
	if err == nil {
		t.Error("expected error when registry is nil, got nil")
	}
}
