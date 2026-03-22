package main

import (
	"testing"

	"github.com/scttfrdmn/strata/internal/scan"
)

func TestParseLuaContent_BaseVariable(t *testing.T) {
	content := `local base = "/opt/python/3.11.11"
prepend_path("PATH", pathJoin(base, "bin"))
`
	var p scan.DetectedPackage
	parseLuaContent(content, &p)
	if p.Prefix != "/opt/python/3.11.11" {
		t.Errorf("Prefix = %q, want /opt/python/3.11.11", p.Prefix)
	}
}

func TestParseLuaContent_RootVariable(t *testing.T) {
	content := `local root = "/strata/env/gcc/13.2.0"
`
	var p scan.DetectedPackage
	parseLuaContent(content, &p)
	if p.Prefix != "/strata/env/gcc/13.2.0" {
		t.Errorf("Prefix = %q, want /strata/env/gcc/13.2.0", p.Prefix)
	}
}

func TestParseLuaContent_PrefixVariable(t *testing.T) {
	content := `local prefix = "/opt/apps/mylib/1.0"
`
	var p scan.DetectedPackage
	parseLuaContent(content, &p)
	if p.Prefix != "/opt/apps/mylib/1.0" {
		t.Errorf("Prefix = %q, want /opt/apps/mylib/1.0", p.Prefix)
	}
}

func TestParseLuaContent_RelativePath_Ignored(t *testing.T) {
	content := `local base = "relative/path"
`
	var p scan.DetectedPackage
	parseLuaContent(content, &p)
	if p.Prefix != "" {
		t.Errorf("expected empty prefix for relative path, got %q", p.Prefix)
	}
}

func TestParseLuaContent_NoMatch(t *testing.T) {
	content := `-- empty module
prepend_path("PATH", "/some/path")
`
	var p scan.DetectedPackage
	parseLuaContent(content, &p)
	if p.Prefix != "" {
		t.Errorf("expected empty Prefix, got %q", p.Prefix)
	}
}

func TestParseTclContent_Root(t *testing.T) {
	content := `#%Module
set root /opt/fftw/3.3.10
prepend-path PATH $root/bin
`
	var p scan.DetectedPackage
	parseTclContent(content, &p)
	if p.Prefix != "/opt/fftw/3.3.10" {
		t.Errorf("Prefix = %q, want /opt/fftw/3.3.10", p.Prefix)
	}
}

func TestParseTclContent_Base(t *testing.T) {
	content := `set base /opt/hdf5/1.14.4
prepend-path PATH $base/bin
`
	var p scan.DetectedPackage
	parseTclContent(content, &p)
	if p.Prefix != "/opt/hdf5/1.14.4" {
		t.Errorf("Prefix = %q, want /opt/hdf5/1.14.4", p.Prefix)
	}
}

func TestParseTclContent_Prefix(t *testing.T) {
	content := `set prefix /opt/lmod/8.7.37
`
	var p scan.DetectedPackage
	parseTclContent(content, &p)
	if p.Prefix != "/opt/lmod/8.7.37" {
		t.Errorf("Prefix = %q, want /opt/lmod/8.7.37", p.Prefix)
	}
}

func TestParseTclContent_RelativePath_Ignored(t *testing.T) {
	content := `set root relative/path
`
	var p scan.DetectedPackage
	parseTclContent(content, &p)
	if p.Prefix != "" {
		t.Errorf("expected empty prefix for relative path, got %q", p.Prefix)
	}
}

func TestParseTclContent_NoMatch(t *testing.T) {
	var p scan.DetectedPackage
	parseTclContent("# no set commands here\n", &p)
	if p.Prefix != "" {
		t.Errorf("expected empty Prefix, got %q", p.Prefix)
	}
}

func TestResolveLmodPrefix_InvalidFormat(t *testing.T) {
	_, _, _, err := resolveLmodPrefix("no-slash")
	if err == nil {
		t.Fatal("expected error for invalid module spec format")
	}
}

func TestResolveLmodPrefix_NoModulepath(t *testing.T) {
	t.Setenv("MODULEPATH", "")
	_, _, _, err := resolveLmodPrefix("python/3.11.11")
	if err == nil {
		t.Fatal("expected error when MODULEPATH not set")
	}
}

func TestResolveCondaPrefix_NoPrefixNoEnv(t *testing.T) {
	t.Setenv("CONDA_PREFIX", "")
	t.Setenv("CONDA_EXE", "")
	_, _, _, err := resolveCondaPrefix("numpy", "")
	if err == nil {
		t.Fatal("expected error when no conda prefix available")
	}
}

func TestResolveCondaPrefix_WithEnvNameNoCondaExe(t *testing.T) {
	t.Setenv("CONDA_EXE", "")
	_, _, _, err := resolveCondaPrefix("numpy", "myenv")
	if err == nil {
		t.Fatal("expected error when --conda-env set but CONDA_EXE not set")
	}
}

func TestNewCaptureCmd_Flags(t *testing.T) {
	cmd := newCaptureCmd()
	for _, flag := range []string{"name", "version", "prefix", "from-lmod", "from-conda",
		"conda-env", "abi", "arch", "normalize", "no-sign", "key", "registry",
		"provides", "requires", "dry-run"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not defined on capture command", flag)
		}
	}
}
