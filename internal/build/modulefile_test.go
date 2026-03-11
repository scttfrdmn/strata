package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func TestGenerateModulefile_Basic(t *testing.T) {
	outputDir := t.TempDir()
	installPrefix := filepath.Join(outputDir, "gcc", "14.2.0")
	if err := os.MkdirAll(filepath.Join(installPrefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installPrefix, "lib64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installPrefix, "share", "man"), 0o755); err != nil {
		t.Fatal(err)
	}

	meta := &RecipeMeta{
		Name:        "gcc",
		Version:     "14.2.0",
		Tier:        "core",
		Description: "GNU Compiler Collection 14.2.0",
		ABI:         "linux-gnu-2.34",
		Provides: []spec.Capability{
			{Name: "gcc", Version: "14.2.0"},
			{Name: "gfortran", Version: "14.2.0"},
		},
		ModulefileEnv: []ModuleEnvVar{
			{Var: "GCC_HOME", Path: ""},
			{Var: "CC", Path: "bin/gcc"},
			{Var: "FC", Path: "bin/gfortran"},
		},
	}

	if err := GenerateModulefile(outputDir, installPrefix, meta); err != nil {
		t.Fatalf("GenerateModulefile: %v", err)
	}

	mfPath := filepath.Join(outputDir, "modulefiles", "gcc", "14.2.0.lua")
	data, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatalf("reading modulefile: %v", err)
	}
	content := string(data)

	want := []string{
		`local version = "14.2.0"`,
		`local base    = "/strata/env/gcc/" .. version`,
		`whatis("GNU Compiler Collection 14.2.0")`,
		`conflict("gcc")`,
		`conflict("gfortran")`,
		`prepend_path("PATH", base .. "/bin")`,
		`prepend_path("LD_LIBRARY_PATH", base .. "/lib64")`,
		`prepend_path("MANPATH", base .. "/share/man")`,
		`setenv("GCC_HOME", base)`,
		`setenv("CC", base .. "/bin/gcc")`,
		`setenv("FC", base .. "/bin/gfortran")`,
	}
	for _, w := range want {
		if !strings.Contains(content, w) {
			t.Errorf("modulefile missing %q\ngot:\n%s", w, content)
		}
	}

	// lib/ was not created, so LD_LIBRARY_PATH for lib should not appear before lib64.
	if strings.Contains(content, `base .. "/lib"`) {
		t.Errorf("modulefile should not include /lib (dir not present)")
	}
}

func TestGenerateModulefile_FlatLayout(t *testing.T) {
	outputDir := t.TempDir()
	installPrefix := outputDir

	meta := &RecipeMeta{
		Name:          "glibc",
		Version:       "2.34",
		Tier:          "core",
		ABI:           "linux-gnu-2.34",
		InstallLayout: "flat",
		Provides:      []spec.Capability{{Name: "glibc", Version: "2.34"}},
	}

	if err := GenerateModulefile(outputDir, installPrefix, meta); err != nil {
		t.Fatalf("GenerateModulefile flat: %v", err)
	}

	// No modulefile should be written for flat layouts.
	mfPath := filepath.Join(outputDir, "modulefiles", "glibc", "2.34.lua")
	if _, err := os.Stat(mfPath); err == nil {
		t.Error("modulefile should not be created for flat install layout")
	}
}

func TestGenerateModulefile_NotUserSelectable(t *testing.T) {
	outputDir := t.TempDir()
	installPrefix := filepath.Join(outputDir, "hwloc", "2.11.2")
	if err := os.MkdirAll(filepath.Join(installPrefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	notSelectable := false
	meta := &RecipeMeta{
		Name:           "hwloc",
		Version:        "2.11.2",
		Tier:           "library",
		ABI:            "linux-gnu-2.34",
		UserSelectable: &notSelectable,
		Provides:       []spec.Capability{{Name: "hwloc", Version: "2.11.2"}},
	}

	if err := GenerateModulefile(outputDir, installPrefix, meta); err != nil {
		t.Fatalf("GenerateModulefile: %v", err)
	}

	mfPath := filepath.Join(outputDir, "modulefiles", "hwloc", "2.11.2.lua")
	data, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatalf("reading modulefile: %v", err)
	}
	content := string(data)

	// Non-user-selectable layers should not emit conflict().
	if strings.Contains(content, "conflict") {
		t.Errorf("non-user-selectable modulefile should not contain conflict(); got:\n%s", content)
	}
	// But PATH should still be set.
	if !strings.Contains(content, `prepend_path("PATH"`) {
		t.Errorf("modulefile should prepend PATH; got:\n%s", content)
	}
}

func TestGenerateModulefile_NoSubdirs(t *testing.T) {
	outputDir := t.TempDir()
	installPrefix := filepath.Join(outputDir, "mylib", "1.0.0")
	if err := os.MkdirAll(installPrefix, 0o755); err != nil {
		t.Fatal(err)
	}
	// No subdirectories created — nothing to auto-detect.

	meta := &RecipeMeta{
		Name:     "mylib",
		Version:  "1.0.0",
		Tier:     "library",
		ABI:      "linux-gnu-2.34",
		Provides: []spec.Capability{{Name: "mylib", Version: "1.0.0"}},
	}

	if err := GenerateModulefile(outputDir, installPrefix, meta); err != nil {
		t.Fatalf("GenerateModulefile: %v", err)
	}

	mfPath := filepath.Join(outputDir, "modulefiles", "mylib", "1.0.0.lua")
	if _, err := os.Stat(mfPath); err != nil {
		t.Fatalf("modulefile should exist even with no subdirs: %v", err)
	}
	data, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatal(err)
	}
	// No prepend_path lines expected.
	if strings.Contains(string(data), "prepend_path") {
		t.Errorf("expected no prepend_path with empty install dir; got:\n%s", string(data))
	}
}

func TestGenerateModulefile_WritesCorrectPath(t *testing.T) {
	outputDir := t.TempDir()
	installPrefix := filepath.Join(outputDir, "python", "3.12.13")
	if err := os.MkdirAll(filepath.Join(installPrefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	meta := &RecipeMeta{
		Name:     "python",
		Version:  "3.12.13",
		Tier:     "core",
		ABI:      "linux-gnu-2.34",
		Provides: []spec.Capability{{Name: "python", Version: "3.12.13"}},
	}

	if err := GenerateModulefile(outputDir, installPrefix, meta); err != nil {
		t.Fatalf("GenerateModulefile: %v", err)
	}

	// Verify the file is at the expected path.
	expected := filepath.Join(outputDir, "modulefiles", "python", "3.12.13.lua")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected modulefile at %s: %v", expected, err)
	}
}
