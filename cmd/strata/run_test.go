package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func TestBuildRunEnv_PathAndLD(t *testing.T) {
	lf := &spec.LockFile{
		ProfileName: "test-env",
		RekorEntry:  "42",
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{Name: "python", Version: "3.13.2"},
				MountOrder:    1,
			},
			{
				LayerManifest: spec.LayerManifest{Name: "gcc", Version: "14.2.0"},
				MountOrder:    2,
			},
			{
				LayerManifest: spec.LayerManifest{
					Name:          "glibc",
					Version:       "2.34",
					InstallLayout: "flat",
				},
				MountOrder: 3,
			},
		},
	}

	env := buildRunEnv(lf, "/tmp/strata-test/env", nil)

	pathVal := envVar(env, "PATH")
	if pathVal == "" {
		t.Fatal("PATH not set in env")
	}
	if !strings.Contains(pathVal, "/tmp/strata-test/env/python/3.13.2/bin") {
		t.Errorf("PATH missing python: %s", pathVal)
	}
	if !strings.Contains(pathVal, "/tmp/strata-test/env/gcc/14.2.0/bin") {
		t.Errorf("PATH missing gcc: %s", pathVal)
	}
	// flat layout (glibc) must not appear in PATH
	if strings.Contains(pathVal, "glibc") {
		t.Errorf("PATH must not include flat-layout layer: %s", pathVal)
	}

	ldVal := envVar(env, "LD_LIBRARY_PATH")
	if !strings.Contains(ldVal, "/tmp/strata-test/env/python/3.13.2/lib") {
		t.Errorf("LD_LIBRARY_PATH missing python/lib: %s", ldVal)
	}

	strataEnv := envVar(env, "STRATA_ENV")
	if strataEnv != "/tmp/strata-test/env" {
		t.Errorf("STRATA_ENV: got %q, want /tmp/strata-test/env", strataEnv)
	}
}

func TestBuildRunEnv_MultiVersionOnlyLast(t *testing.T) {
	lf := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.12.13"}, MountOrder: 1},
			{LayerManifest: spec.LayerManifest{Name: "python", Version: "3.13.2"}, MountOrder: 2},
		},
	}
	env := buildRunEnv(lf, "/env", nil)
	path := envVar(env, "PATH")
	if !strings.Contains(path, "/env/python/3.13.2/bin") {
		t.Errorf("PATH missing latest python: %s", path)
	}
	if strings.Contains(path, "/env/python/3.12.13/bin") {
		t.Errorf("PATH must not include older python: %s", path)
	}
}

func TestBuildRunEnv_EnvOverrides(t *testing.T) {
	lf := &spec.LockFile{
		Env: map[string]string{"MY_VAR": "from_lockfile"},
	}
	env := buildRunEnv(lf, "/env", []string{"MY_VAR=from_cli", "EXTRA=extra_val"})
	if envVar(env, "MY_VAR") != "from_cli" {
		t.Errorf("MY_VAR should be from CLI: %s", envVar(env, "MY_VAR"))
	}
	if envVar(env, "EXTRA") != "extra_val" {
		t.Errorf("EXTRA should be extra_val: %s", envVar(env, "EXTRA"))
	}
}

func TestDefaultCacheDir_NonRoot(t *testing.T) {
	// Temporarily clear XDG_CACHE_HOME to test the ~/.cache fallback.
	orig := os.Getenv("XDG_CACHE_HOME")
	os.Unsetenv("XDG_CACHE_HOME")           //nolint:errcheck
	defer os.Setenv("XDG_CACHE_HOME", orig) //nolint:errcheck

	if os.Getuid() == 0 {
		t.Skip("running as root — this tests non-root path")
	}

	dir := defaultCacheDir()
	if !strings.Contains(dir, "strata") {
		t.Errorf("defaultCacheDir should contain 'strata': %s", dir)
	}
	if !strings.HasSuffix(dir, "layers") {
		t.Errorf("defaultCacheDir should end in 'layers': %s", dir)
	}
}

func TestDefaultCacheDir_XDG(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root")
	}
	os.Setenv("XDG_CACHE_HOME", "/custom/cache") //nolint:errcheck
	defer os.Unsetenv("XDG_CACHE_HOME")          //nolint:errcheck

	dir := defaultCacheDir()
	want := filepath.Join("/custom/cache", "strata", "layers")
	if dir != want {
		t.Errorf("defaultCacheDir: got %q, want %q", dir, want)
	}
}

func TestParseS3URIRun(t *testing.T) {
	tests := []struct {
		uri    string
		bucket string
		key    string
		ok     bool
	}{
		{"s3://my-bucket/path/to/key.sqfs", "my-bucket", "path/to/key.sqfs", true},
		{"s3://bucket-only", "bucket-only", "", true},
		{"file:///tmp/foo.sqfs", "", "", false},
		{"not-a-uri", "", "", false},
	}
	for _, tt := range tests {
		bucket, key, ok := parseS3URIRun(tt.uri)
		if ok != tt.ok || bucket != tt.bucket || key != tt.key {
			t.Errorf("parseS3URIRun(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.uri, bucket, key, ok, tt.bucket, tt.key, tt.ok)
		}
	}
}

// envVar finds the value of KEY in a KEY=VAL slice.
func envVar(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}
