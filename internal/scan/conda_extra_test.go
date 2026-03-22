package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeCondaChannel_URL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://conda.anaconda.org/conda-forge/linux-64", "conda-forge"},
		{"https://repo.anaconda.com/pkgs/main/linux-aarch64", "main"},
		{"https://conda.anaconda.org/bioconda/osx-64", "bioconda"},
		{"conda-forge", "conda-forge"},
		{"", ""},
		{"https://conda.anaconda.org/defaults/win-64", "defaults"},
		{"https://conda.anaconda.org/nvidia/linux-64", "nvidia"},
	}
	for _, tt := range tests {
		got := normalizeCondaChannel(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCondaChannel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestScanCondaEnvsDir(t *testing.T) {
	tmp := t.TempDir()

	// scanCondaEnvsDir passes ALL subdirs of envs/ to add — filtering by
	// conda-meta presence is the caller's responsibility (see findCondaEnvs).
	for _, d := range []string{
		filepath.Join(tmp, "envs", "myenv", "conda-meta"),
		filepath.Join(tmp, "envs", "otherenv", "conda-meta"),
		filepath.Join(tmp, "envs", "not-an-env"), // no conda-meta — still passed to add
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	var collected []string
	add := func(p string) {
		collected = append(collected, p)
	}
	scanCondaEnvsDir(tmp, add)

	// All 3 subdirs are passed to add (filtering happens in the caller).
	if len(collected) != 3 {
		t.Errorf("expected 3 dirs passed to add, got %d: %v", len(collected), collected)
	}
}

func TestScanCondaEnvsDir_WithFilter(t *testing.T) {
	// Verify the filtering pattern used by findCondaEnvs works correctly.
	tmp := t.TempDir()
	for _, d := range []string{
		filepath.Join(tmp, "envs", "valid-env", "conda-meta"),
		filepath.Join(tmp, "envs", "empty-dir"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate the filtering done inside findCondaEnvs.
	var kept []string
	add := func(p string) {
		if _, err := os.Stat(filepath.Join(p, "conda-meta")); err == nil {
			kept = append(kept, p)
		}
	}
	scanCondaEnvsDir(tmp, add)

	if len(kept) != 1 {
		t.Errorf("expected 1 env with conda-meta, got %d: %v", len(kept), kept)
	}
}

func TestScanCondaEnvsDir_NonexistentRoot(t *testing.T) {
	// Should silently return without error.
	var collected []string
	scanCondaEnvsDir("/nonexistent/conda/root", func(p string) {
		collected = append(collected, p)
	})
	if len(collected) != 0 {
		t.Errorf("expected 0 results for nonexistent root, got %d", len(collected))
	}
}

func TestReadCondaEnv_ParsesEntries(t *testing.T) {
	tmp := t.TempDir()
	metaDir := filepath.Join(tmp, "conda-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeEntry := func(name, version, channel string) {
		entry := condaMetaEntry{Name: name, Version: version, Channel: channel}
		data, _ := json.Marshal(entry)
		_ = os.WriteFile(filepath.Join(metaDir, name+"-"+version+".json"), data, 0644)
	}
	writeEntry("numpy", "1.26.4", "https://conda.anaconda.org/conda-forge/linux-64")
	writeEntry("scipy", "1.12.0", "https://conda.anaconda.org/conda-forge/linux-64")
	// Malformed JSON — should be skipped.
	_ = os.WriteFile(filepath.Join(metaDir, "bad.json"), []byte("{invalid"), 0644)
	// Missing name — should be skipped.
	_ = os.WriteFile(filepath.Join(metaDir, "empty.json"), []byte(`{"name":"","version":"1.0"}`), 0644)

	pkgs := readCondaEnv(tmp)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d: %+v", len(pkgs), pkgs)
	}

	// Verify channel normalization.
	for _, p := range pkgs {
		if p.Channel != "conda-forge" {
			t.Errorf("Channel = %q, want conda-forge", p.Channel)
		}
		if p.Source != SourceConda {
			t.Errorf("Source = %v, want SourceConda", p.Source)
		}
		if p.Prefix != tmp {
			t.Errorf("Prefix = %q, want %q", p.Prefix, tmp)
		}
	}
}

func TestReadCondaEnv_NonexistentDir(t *testing.T) {
	pkgs := readCondaEnv("/nonexistent/env")
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages for nonexistent dir, got %d", len(pkgs))
	}
}

func TestFindCondaEnvs_NoEnvVars(t *testing.T) {
	// Unset CONDA_PREFIX and CONDA_EXE — should still run without panicking.
	// (The well-known paths likely don't exist in CI/test env either.)
	t.Setenv("CONDA_PREFIX", "")
	t.Setenv("CONDA_EXE", "")
	// This may return paths that exist (e.g. ~/miniforge3) — just verify it doesn't panic.
	_ = findCondaEnvs()
}

func TestFindCondaEnvs_WithCondaPrefix(t *testing.T) {
	tmp := t.TempDir()
	// Create a fake conda env at tmp.
	if err := os.MkdirAll(filepath.Join(tmp, "conda-meta"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONDA_PREFIX", tmp)
	t.Setenv("CONDA_EXE", "")

	envs := findCondaEnvs()
	found := false
	for _, e := range envs {
		if e == tmp {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in findCondaEnvs result, got %v", tmp, envs)
	}
}
