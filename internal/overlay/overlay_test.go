package overlay_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// testLockFile returns a minimal LockFile suitable for ConfigureEnvironment tests.
func testLockFile() *spec.LockFile {
	return &spec.LockFile{
		ProfileName: "test-profile",
		RekorEntry:  "42",
		ResolvedAt:  time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:      "python-3.13.2-linux-gnu-2.34-x86_64",
					Name:    "python",
					Version: "3.13.2",
					SHA256:  "abc123",
				},
				MountOrder: 1,
			},
			{
				LayerManifest: spec.LayerManifest{
					ID:            "glibc-2.34-linux-gnu-2.34-x86_64",
					Name:          "glibc",
					Version:       "2.34",
					SHA256:        "def456",
					InstallLayout: "flat",
				},
				MountOrder: 2,
			},
		},
		Env: map[string]string{
			"MY_VAR":    "my_value",
			"TOOL_HOME": "/strata/env/opt/tool",
		},
	}
}

func TestConfigureEnvironment_WritesFiles(t *testing.T) {
	root := t.TempDir()
	lf := testLockFile()
	ov := &overlay.Overlay{
		MergedPath: "/strata/env",
		UpperDir:   "/strata/upper",
		WorkDir:    "/strata/work",
	}

	if err := overlay.ConfigureEnvironment(lf, ov, root); err != nil {
		t.Fatalf("ConfigureEnvironment: %v", err)
	}

	// Verify strata.sh contains the expected keys.
	shPath := filepath.Join(root, "etc", "profile.d", "strata.sh")
	shData, err := os.ReadFile(shPath)
	if err != nil {
		t.Fatalf("reading strata.sh: %v", err)
	}
	sh := string(shData)

	for _, want := range []string{
		"PATH",
		"LD_LIBRARY_PATH",
		"STRATA_PROFILE",
		"STRATA_REKOR_ENTRY",
		"MY_VAR",
		"TOOL_HOME",
		"/strata/env/python/3.13.2/bin", // per-layer versioned path
		"test-profile",
		"42",
	} {
		if !strings.Contains(sh, want) {
			t.Errorf("strata.sh missing %q\ncontent:\n%s", want, sh)
		}
	}
	// Flat-layout layers (glibc) must NOT appear in PATH.
	if strings.Contains(sh, "/strata/env/glibc/") {
		t.Error("strata.sh: flat-layout layer (glibc) must not appear in PATH")
	}
	if !strings.Contains(sh, "export PATH") {
		t.Error("strata.sh: PATH not exported")
	}
	if !strings.Contains(sh, "export LD_LIBRARY_PATH") {
		t.Error("strata.sh: LD_LIBRARY_PATH not exported")
	}

	// Verify /etc/strata/environment (systemd EnvironmentFile format).
	envPath := filepath.Join(root, "etc", "strata", "environment")
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading environment: %v", err)
	}
	envContent := string(envData)
	for _, want := range []string{"PATH=", "LD_LIBRARY_PATH=", "STRATA_PROFILE=", "MY_VAR="} {
		if !strings.Contains(envContent, want) {
			t.Errorf("environment file missing %q", want)
		}
	}
	// systemd EnvironmentFile must not have "export" keyword.
	if strings.Contains(envContent, "export ") {
		t.Error("environment file must not contain 'export' keyword (systemd format)")
	}

	// Verify active.lock.yaml round-trips correctly.
	lockPath := filepath.Join(root, "etc", "strata", "active.lock.yaml")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("reading active.lock.yaml: %v", err)
	}
	var got spec.LockFile
	if err := yaml.Unmarshal(lockData, &got); err != nil {
		t.Fatalf("unmarshal active.lock.yaml: %v", err)
	}
	if got.ProfileName != lf.ProfileName {
		t.Errorf("profile name: got %q, want %q", got.ProfileName, lf.ProfileName)
	}
	if got.RekorEntry != lf.RekorEntry {
		t.Errorf("rekor entry: got %q, want %q", got.RekorEntry, lf.RekorEntry)
	}
	if len(got.Layers) != 2 || got.Layers[0].ID != "python-3.13.2-linux-gnu-2.34-x86_64" {
		t.Errorf("layers not preserved in active.lock.yaml")
	}
}

func TestConfigureEnvironment_NilOverlay(t *testing.T) {
	root := t.TempDir()
	lf := testLockFile()

	// nil Overlay should use the default /strata/env path.
	if err := overlay.ConfigureEnvironment(lf, nil, root); err != nil {
		t.Fatalf("ConfigureEnvironment with nil overlay: %v", err)
	}

	shData, err := os.ReadFile(filepath.Join(root, "etc", "profile.d", "strata.sh"))
	if err != nil {
		t.Fatalf("reading strata.sh: %v", err)
	}
	if !strings.Contains(string(shData), "/strata/env") {
		t.Error("strata.sh: expected default /strata/env path")
	}
}

func TestConfigureEnvironment_MultiVersionPath(t *testing.T) {
	root := t.TempDir()
	lf := &spec.LockFile{
		ProfileName: "multi-test",
		RekorEntry:  "1",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.12.13",
				},
				MountOrder: 1,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.13.2",
				},
				MountOrder: 2,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:    "gcc",
					Version: "14.2.0",
				},
				MountOrder: 3,
			},
		},
	}

	if err := overlay.ConfigureEnvironment(lf, nil, root); err != nil {
		t.Fatalf("ConfigureEnvironment: %v", err)
	}
	shData, err := os.ReadFile(filepath.Join(root, "etc", "profile.d", "strata.sh"))
	if err != nil {
		t.Fatalf("reading strata.sh: %v", err)
	}
	sh := string(shData)

	// Only the last (highest mount_order) python version should appear in PATH.
	if !strings.Contains(sh, "/strata/env/python/3.13.2/bin") {
		t.Error("strata.sh: missing latest python version in PATH")
	}
	if strings.Contains(sh, "/strata/env/python/3.12.13/bin") {
		t.Error("strata.sh: older python version must not appear in PATH (lmod handles switching)")
	}
	// Single-version gcc must still appear.
	if !strings.Contains(sh, "/strata/env/gcc/14.2.0/bin") {
		t.Error("strata.sh: missing gcc in PATH")
	}
}

func TestMountReturnsErrNotSupportedOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("stub not active on Linux")
	}
	_, err := overlay.Mount(nil)
	if err != overlay.ErrNotSupported {
		t.Errorf("Mount: got %v, want ErrNotSupported", err)
	}
}

func TestCleanupReturnsErrNotSupportedOnNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("stub not active on Linux")
	}
	var o *overlay.Overlay
	err := o.Cleanup()
	if err != overlay.ErrNotSupported {
		t.Errorf("Cleanup on nil Overlay: got %v, want ErrNotSupported", err)
	}
}
