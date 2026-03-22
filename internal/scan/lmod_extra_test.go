package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLuaModulefile_BaseVariable(t *testing.T) {
	content := `local base = "/opt/python/3.11.11"
prepend_path("PATH", pathJoin(base, "bin"))
`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if p.Prefix != "/opt/python/3.11.11" {
		t.Errorf("Prefix = %q, want /opt/python/3.11.11", p.Prefix)
	}
}

func TestParseLuaModulefile_RootVariable(t *testing.T) {
	content := `local root = "/strata/env/gcc/13.2.0"
prepend_path("PATH", pathJoin(root, "bin"))
`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if p.Prefix != "/strata/env/gcc/13.2.0" {
		t.Errorf("Prefix = %q, want /strata/env/gcc/13.2.0", p.Prefix)
	}
}

func TestParseLuaModulefile_PrefixVariable(t *testing.T) {
	content := `local prefix = "/opt/apps/lmod/8.7.37"
`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if p.Prefix != "/opt/apps/lmod/8.7.37" {
		t.Errorf("Prefix = %q, want /opt/apps/lmod/8.7.37", p.Prefix)
	}
}

func TestParseLuaModulefile_HomeEnvFallback(t *testing.T) {
	content := `setenv("PYTHON_HOME", "/opt/python/3.11.11")
`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if p.Prefix != "/opt/python/3.11.11" {
		t.Errorf("Prefix (HOME fallback) = %q, want /opt/python/3.11.11", p.Prefix)
	}
}

func TestParseLuaModulefile_Prereqs(t *testing.T) {
	content := `local base = "/opt/openmpi/5.0.10"
prereq("gcc/13.2.0")
depends_on("hwloc/2.11.2")
`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if len(p.ModuleDeps) != 2 {
		t.Fatalf("ModuleDeps = %v, want 2 entries", p.ModuleDeps)
	}
	if p.ModuleDeps[0] != "gcc/13.2.0" {
		t.Errorf("ModuleDeps[0] = %q, want gcc/13.2.0", p.ModuleDeps[0])
	}
	if p.ModuleDeps[1] != "hwloc/2.11.2" {
		t.Errorf("ModuleDeps[1] = %q, want hwloc/2.11.2", p.ModuleDeps[1])
	}
}

func TestParseLuaModulefile_NoMatch(t *testing.T) {
	content := `-- empty module with no prefix\n`
	var p DetectedPackage
	parseLuaModulefile(content, &p)
	if p.Prefix != "" {
		t.Errorf("expected empty Prefix, got %q", p.Prefix)
	}
}

func TestParseTclModulefile_Root(t *testing.T) {
	content := `#%Module
set root /opt/fftw/3.3.10
prepend-path PATH $root/bin
`
	var p DetectedPackage
	parseTclModulefile(content, &p)
	if p.Prefix != "/opt/fftw/3.3.10" {
		t.Errorf("Prefix = %q, want /opt/fftw/3.3.10", p.Prefix)
	}
}

func TestParseTclModulefile_Base(t *testing.T) {
	content := `set base /opt/hdf5/1.14.4
`
	var p DetectedPackage
	parseTclModulefile(content, &p)
	if p.Prefix != "/opt/hdf5/1.14.4" {
		t.Errorf("Prefix = %q, want /opt/hdf5/1.14.4", p.Prefix)
	}
}

func TestParseTclModulefile_Prereqs(t *testing.T) {
	content := `set root /opt/openmpi/5.0.10
prereq gcc/13.2.0
prereq hwloc
`
	var p DetectedPackage
	parseTclModulefile(content, &p)
	if len(p.ModuleDeps) != 2 {
		t.Fatalf("ModuleDeps = %v, want 2 entries", p.ModuleDeps)
	}
}

func TestWalkModuleDir_LuaFiles(t *testing.T) {
	tmp := t.TempDir()
	// Create <tmp>/python/3.11.11.lua
	pkgDir := filepath.Join(tmp, "python")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	luaContent := `local base = "/opt/python/3.11.11"
prepend_path("PATH", pathJoin(base, "bin"))
`
	if err := os.WriteFile(filepath.Join(pkgDir, "3.11.11.lua"), []byte(luaContent), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs := walkModuleDir(tmp)
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "python" {
		t.Errorf("Name = %q, want python", pkgs[0].Name)
	}
	if pkgs[0].Version != "3.11.11" {
		t.Errorf("Version = %q, want 3.11.11", pkgs[0].Version)
	}
	if pkgs[0].Prefix != "/opt/python/3.11.11" {
		t.Errorf("Prefix = %q, want /opt/python/3.11.11", pkgs[0].Prefix)
	}
}

func TestWalkModuleDir_TclFiles(t *testing.T) {
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "fftw")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	tclContent := "#%Module\nset root /opt/fftw/3.3.10\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "3.3.10.tcl"), []byte(tclContent), 0644); err != nil {
		t.Fatal(err)
	}

	pkgs := walkModuleDir(tmp)
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Name != "fftw" || pkgs[0].Version != "3.3.10" {
		t.Errorf("got %s@%s, want fftw@3.3.10", pkgs[0].Name, pkgs[0].Version)
	}
}

func TestWalkModuleDir_NonexistentDir(t *testing.T) {
	pkgs := walkModuleDir("/nonexistent/path/that/does/not/exist")
	if len(pkgs) != 0 {
		t.Errorf("expected empty result for nonexistent dir, got %d packages", len(pkgs))
	}
}
