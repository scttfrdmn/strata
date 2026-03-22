package scan

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseLuaModulefile(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(file), "testdata", "lmod")

	t.Run("python", func(t *testing.T) {
		p := parseModulefile("python", "3.11.11", filepath.Join(testdata, "python", "3.11.11.lua"))
		if p.Prefix != "/opt/apps/python/3.11.11" {
			t.Errorf("Prefix = %q, want /opt/apps/python/3.11.11", p.Prefix)
		}
		if p.Name != "python" {
			t.Errorf("Name = %q, want python", p.Name)
		}
		if p.Version != "3.11.11" {
			t.Errorf("Version = %q, want 3.11.11", p.Version)
		}
	})

	t.Run("gcc_with_prereq", func(t *testing.T) {
		p := parseModulefile("gcc", "13.2.0", filepath.Join(testdata, "gcc", "13.2.0.lua"))
		if p.Prefix != "/opt/apps/gcc/13.2.0" {
			t.Errorf("Prefix = %q, want /opt/apps/gcc/13.2.0", p.Prefix)
		}
		if len(p.ModuleDeps) == 0 || p.ModuleDeps[0] != "binutils" {
			t.Errorf("ModuleDeps = %v, want [binutils]", p.ModuleDeps)
		}
	})

	t.Run("openmpi_tcl", func(t *testing.T) {
		p := parseModulefile("openmpi", "5.0.10", filepath.Join(testdata, "openmpi", "5.0.10.tcl"))
		if p.Prefix != "/opt/apps/openmpi/5.0.10" {
			t.Errorf("Prefix = %q, want /opt/apps/openmpi/5.0.10", p.Prefix)
		}
		if len(p.ModuleDeps) == 0 || p.ModuleDeps[0] != "gcc" {
			t.Errorf("ModuleDeps = %v, want [gcc]", p.ModuleDeps)
		}
	})
}
