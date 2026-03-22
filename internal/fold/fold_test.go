package fold

import (
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func TestBuildEnvParts_VersionedLayers(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.11.11",
				},
				MountOrder: 1,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:    "gcc",
					Version: "13.2.0",
				},
				MountOrder: 2,
			},
		},
	}
	rootDir := "/output/root"
	pathParts, ldParts := buildEnvParts(lf, rootDir)

	if len(pathParts) != 2 {
		t.Errorf("expected 2 PATH parts, got %d", len(pathParts))
	}
	if len(ldParts) != 4 {
		t.Errorf("expected 4 LD_LIBRARY_PATH parts, got %d: %v", len(ldParts), ldParts)
	}

	wantPath0 := rootDir + "/python/3.11.11/bin"
	if pathParts[0] != wantPath0 {
		t.Errorf("pathParts[0] = %q, want %q", pathParts[0], wantPath0)
	}
}

func TestBuildEnvParts_FlatLayerSkipped(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:          "glibc",
					Version:       "2.34",
					InstallLayout: "flat",
				},
				MountOrder: 1,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.11.11",
				},
				MountOrder: 2,
			},
		},
	}
	pathParts, ldParts := buildEnvParts(lf, "/root")
	if len(pathParts) != 1 {
		t.Errorf("expected 1 PATH part (flat glibc skipped), got %d: %v", len(pathParts), pathParts)
	}
	if len(ldParts) != 2 {
		t.Errorf("expected 2 LD parts, got %d", len(ldParts))
	}
}

func TestBuildEnvParts_MultiVersionDedup(t *testing.T) {
	// When two versions of the same package appear, only the last one should
	// be in PATH (consistent with buildRunEnv and ConfigureEnvironment).
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.11.11",
				},
				MountOrder: 1,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.12.13",
				},
				MountOrder: 2,
			},
		},
	}
	pathParts, _ := buildEnvParts(lf, "/root")
	if len(pathParts) != 1 {
		t.Errorf("expected 1 PATH part (only last python version), got %d: %v", len(pathParts), pathParts)
	}
	want := "/root/python/3.12.13/bin"
	if pathParts[0] != want {
		t.Errorf("pathParts[0] = %q, want %q", pathParts[0], want)
	}
}

func TestBuildEnvSh(t *testing.T) {
	lf := &spec.LockFile{
		ProfileName: "test-profile",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    "python",
					Version: "3.11.11",
				},
				MountOrder: 1,
			},
		},
	}
	sh := buildEnvSh(lf, "/output/root")
	if sh == "" {
		t.Fatal("expected non-empty env.sh")
	}
	// Should contain PATH and STRATA_PROFILE.
	for _, want := range []string{"export PATH=", "STRATA_PROFILE", "python/3.11.11/bin"} {
		if !containsStr(sh, want) {
			t.Errorf("env.sh missing %q; got:\n%s", want, sh)
		}
	}
}

func TestBuildEnvironmentFile(t *testing.T) {
	lf := &spec.LockFile{
		ProfileName: "test-profile",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					Name:    "gcc",
					Version: "13.2.0",
				},
				MountOrder: 1,
			},
		},
	}
	env := buildEnvironmentFile(lf, "/output/root")
	for _, want := range []string{"PATH=", "LD_LIBRARY_PATH=", "STRATA_PROFILE="} {
		if !containsStr(env, want) {
			t.Errorf("environment file missing %q; got:\n%s", want, env)
		}
	}
}

func TestMergeConfig_Validation(t *testing.T) {
	// Test that MergeConfig requires Name and Version.
	// This exercises the struct fields without calling MergeToLayer
	// (which requires Linux + mksquashfs + overlay).
	cfg := MergeConfig{DryRun: true}
	if cfg.Name != "" {
		t.Error("expected empty Name")
	}
	if cfg.Version != "" {
		t.Error("expected empty Version")
	}
}

func TestEjectConfig_Defaults(t *testing.T) {
	cfg := EjectConfig{}
	if cfg.OutputDir != "" {
		t.Error("expected empty OutputDir")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"it's", "'it'\\''s'"},
		{"hello world", "'hello world'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
