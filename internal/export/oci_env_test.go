package export

import (
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

func TestBuildOCIEnv_PathAndLD(t *testing.T) {
	lf := &spec.LockFile{
		ProfileName: "test",
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.11.11"}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{Name: "gcc", Version: "13.2.0"}, MountOrder: 2},
		},
	}
	env := buildOCIEnv(lf)

	envMap := parseEnvSlice(env)

	path, ok := envMap["PATH"]
	if !ok {
		t.Fatal("PATH not set in OCI env")
	}
	if !strings.Contains(path, "/strata/env/python/3.11.11/bin") {
		t.Errorf("PATH missing python bin: %s", path)
	}
	if !strings.Contains(path, "/strata/env/gcc/13.2.0/bin") {
		t.Errorf("PATH missing gcc bin: %s", path)
	}
	if !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("PATH missing system bins: %s", path)
	}

	ld, ok := envMap["LD_LIBRARY_PATH"]
	if !ok {
		t.Fatal("LD_LIBRARY_PATH not set in OCI env")
	}
	if !strings.Contains(ld, "/strata/env/python/3.11.11/lib") {
		t.Errorf("LD_LIBRARY_PATH missing python lib: %s", ld)
	}

	if envMap["STRATA_PROFILE"] != "test" {
		t.Errorf("STRATA_PROFILE = %q, want test", envMap["STRATA_PROFILE"])
	}
	if envMap["STRATA_ENV"] != "/strata/env" {
		t.Errorf("STRATA_ENV = %q, want /strata/env", envMap["STRATA_ENV"])
	}
}

func TestBuildOCIEnv_FlatLayerSkipped(t *testing.T) {
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
				LayerManifest: spec.LayerManifest{Name: "python", Version: "3.11.11"},
				MountOrder:    2,
			},
		},
	}
	env := buildOCIEnv(lf)
	envMap := parseEnvSlice(env)

	path := envMap["PATH"]
	if strings.Contains(path, "glibc") {
		t.Errorf("PATH should not include flat-layout glibc: %s", path)
	}
	if !strings.Contains(path, "python") {
		t.Errorf("PATH should include python: %s", path)
	}
}

func TestBuildOCIEnv_MultiVersionDedup(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.11.11"}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.12.13"}, MountOrder: 2},
		},
	}
	env := buildOCIEnv(lf)
	envMap := parseEnvSlice(env)

	path := envMap["PATH"]
	if strings.Contains(path, "3.11.11") {
		t.Errorf("PATH should not include older python 3.11.11: %s", path)
	}
	if !strings.Contains(path, "3.12.13") {
		t.Errorf("PATH should include latest python 3.12.13: %s", path)
	}
}

func TestBuildOCIEnv_LockfileEnvVars(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.11.11"}, MountOrder: 1},
		},
		Env: map[string]string{
			"MY_CUSTOM_VAR": "hello",
		},
	}
	env := buildOCIEnv(lf)
	envMap := parseEnvSlice(env)
	if envMap["MY_CUSTOM_VAR"] != "hello" {
		t.Errorf("MY_CUSTOM_VAR = %q, want hello", envMap["MY_CUSTOM_VAR"])
	}
}

func TestBuildOCILabels_EnvironmentID(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:     "python-3.11.11-linux-gnu-2.34-x86_64",
					Name:   "python",
					SHA256: "abc123",
				},
				MountOrder: 1,
			},
		},
	}
	lp := []overlay.LayerPath{
		{ID: "python-3.11.11-linux-gnu-2.34-x86_64", SHA256: "abc123"},
	}

	labels := buildOCILabels(lf, lp)

	if _, ok := labels["org.opencontainers.image.revision"]; !ok {
		t.Error("missing org.opencontainers.image.revision label")
	}
	if labels["strata.layer.python.sha256"] != "abc123" {
		t.Errorf("strata.layer.python.sha256 = %q, want abc123", labels["strata.layer.python.sha256"])
	}
}

func TestBuildOCILabels_RekorEntry(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "12345",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:         "gcc-13.2.0",
					Name:       "gcc",
					SHA256:     "deadbeef",
					RekorEntry: "99",
				},
				MountOrder: 1,
			},
		},
	}
	lp := []overlay.LayerPath{{ID: "gcc-13.2.0", SHA256: "deadbeef"}}

	labels := buildOCILabels(lf, lp)

	if labels["strata.lockfile.rekor_entry"] != "12345" {
		t.Errorf("lockfile rekor_entry = %q, want 12345", labels["strata.lockfile.rekor_entry"])
	}
	if labels["strata.layer.gcc.rekor_entry"] != "99" {
		t.Errorf("layer rekor_entry = %q, want 99", labels["strata.layer.gcc.rekor_entry"])
	}
}

func TestBuildOCILabels_NoRekorEntry(t *testing.T) {
	lf := &spec.LockFile{}
	labels := buildOCILabels(lf, nil)

	if _, ok := labels["strata.lockfile.rekor_entry"]; ok {
		t.Error("should not include rekor_entry label when empty")
	}
}

// parseEnvSlice converts []string{"KEY=VAL"} to map[string]string.
func parseEnvSlice(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			continue
		}
		m[e[:idx]] = e[idx+1:]
	}
	return m
}
