package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFoldCmd_Flags(t *testing.T) {
	cmd := newFoldCmd()

	// Verify required flags are defined.
	for _, name := range []string{"lockfile", "name", "version", "eject", "abi", "arch",
		"no-sign", "key", "registry", "provides", "requires", "cache-dir", "dry-run"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not defined on fold command", name)
		}
	}
}

func TestRunFold_NoLayers(t *testing.T) {
	// Write a lockfile with no layers.
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "empty.lock.yaml")
	content := `profile: test
layers: []
base:
  declared_os: al2023
  ami_id: ami-000
  ami_sha256: ""
  capabilities:
    ami_id: ami-000
    os: al2023
    arch: x86_64
    abi: linux-gnu-2.34
    probed_at: "2024-01-01T00:00:00Z"
    system_compiler: gcc-11
    provides: []
`
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runFold(t.Context(), lockPath, "test", "1.0.0", "", "", "", "", "", "", "", tmp, true, false)
	if err == nil {
		t.Fatal("expected error for lockfile with no layers")
	}
	if err.Error() != "fold: lockfile has no layers" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunFold_DryRun_MissingNameVersion(t *testing.T) {
	// Dry-run with --eject and no layers still requires a lockfile to parse.
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "test.lock.yaml")
	content := `profile: test
layers:
  - name: python
    version: "3.11.11"
    id: python-3.11.11-linux-gnu-2.34-x86_64
    sha256: abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    source: "file:///dev/null"
    arch: x86_64
    abi: linux-gnu-2.34
    mount_order: 1
base:
  declared_os: al2023
  ami_id: ami-000
  ami_sha256: ""
  capabilities:
    ami_id: ami-000
    os: al2023
    arch: x86_64
    abi: linux-gnu-2.34
    probed_at: "2024-01-01T00:00:00Z"
    system_compiler: gcc-11
    provides: []
`
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Dry run without --eject should require --name.
	err := runFold(t.Context(), lockPath, "", "1.0.0", "", "", "", "", "", "", "", tmp, true, true)
	if err == nil {
		t.Fatal("expected error for missing --name in dry-run")
	}
}

func TestRunFold_DryRun_EjectMode(t *testing.T) {
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "test.lock.yaml")
	content := `profile: test
layers:
  - name: python
    version: "3.11.11"
    id: python-3.11.11-linux-gnu-2.34-x86_64
    sha256: abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    source: "file:///dev/null"
    arch: x86_64
    abi: linux-gnu-2.34
    mount_order: 1
base:
  declared_os: al2023
  ami_id: ami-000
  ami_sha256: ""
  capabilities:
    ami_id: ami-000
    os: al2023
    arch: x86_64
    abi: linux-gnu-2.34
    probed_at: "2024-01-01T00:00:00Z"
    system_compiler: gcc-11
    provides: []
`
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ejectDir := filepath.Join(tmp, "output")
	// Eject dry-run should succeed (just prints plan).
	err := runFold(t.Context(), lockPath, "", "", ejectDir, "", "", "", "", "", "", tmp, true, true)
	if err != nil {
		t.Errorf("unexpected error in eject dry-run: %v", err)
	}
}
