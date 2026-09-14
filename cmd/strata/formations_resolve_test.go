package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestShippedFormationsResolveOffline is #108's closing condition: every
// formation the binary ships must resolve against the embedded catalog — the
// on-ramp a user with a fresh clone and no AWS actually hits. Before #108 none
// of the six did: the catalog carries no Sigstore bundles and stage 7 refused
// every layer, and hpc-mpi failed even earlier at stage 4 for want of
// ucx/hwloc/pmix/libfabric. catalog_test.go only asserts the YAML parses;
// nothing constructed a resolver over the shipped catalog, which is the whole gap
// this test closes.
func TestShippedFormationsResolveOffline(t *testing.T) {
	clearAWSEnv(t)
	t.Setenv("STRATA_REGISTRY_URL", "") // force the embedded, unsigned offline catalog

	entries, err := os.ReadDir("formations")
	if err != nil {
		t.Fatalf("read formations dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Fatal("no shipped formations found; the path or fixture is wrong")
	}

	for _, file := range files {
		nameVer := strings.TrimSuffix(file, ".yaml") // e.g. "hpc-mpi@2026.03"
		t.Run(nameVer, func(t *testing.T) {
			work := t.TempDir()
			profile := filepath.Join(work, "probe.yaml")
			content := "name: probe\ndescription: probe\nversion: \"1.0\"\n" +
				"base:\n  os: al2023\n  arch: x86_64\n" +
				"software:\n  - formation: " + nameVer + "\n" +
				"instance:\n  type: c7i.large\n"
			if err := os.WriteFile(profile, []byte(content), 0o644); err != nil {
				t.Fatalf("write profile: %v", err)
			}
			out := filepath.Join(work, "out.lock.yaml")

			cmd := newResolveCmd()
			cmd.SetArgs([]string{profile, "-o", out})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("resolving %s against the embedded catalog failed: %v", nameVer, err)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("%s: no lockfile written: %v", nameVer, err)
			}
			lf, err := spec.ParseLockFileBytes(data)
			if err != nil {
				t.Fatalf("%s: parsing lockfile: %v", nameVer, err)
			}
			if len(lf.Layers) == 0 {
				t.Errorf("%s: lockfile has no layers", nameVer)
			}
		})
	}
}

// TestSingleLayerResolvesOffline is #54's half: the shipped-catalog offline path
// must resolve a plain (non-formation) profile too. Before #108, stage 7 refused
// *every* profile against the embedded catalog because its recipe-derived layers
// carry no bundle; now the offline path warns and resolves.
func TestSingleLayerResolvesOffline(t *testing.T) {
	clearAWSEnv(t)
	t.Setenv("STRATA_REGISTRY_URL", "")

	work := t.TempDir()
	profile := filepath.Join(work, "single.yaml")
	content := "name: probe-single\ndescription: probe\nversion: \"1.0\"\n" +
		"base:\n  os: al2023\n  arch: x86_64\n" +
		"software:\n  - python@3.13.2\n" +
		"instance:\n  type: c7i.large\n"
	if err := os.WriteFile(profile, []byte(content), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	out := filepath.Join(work, "single.lock.yaml")

	cmd := newResolveCmd()
	cmd.SetArgs([]string{profile, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("single-layer offline resolve failed: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("no lockfile written: %v", err)
	}
}

// TestShippedExamplesResolveOffline is #70's closing condition: every profile in
// examples/ must resolve against the shipped catalog. Before, all three named a
// formation version the catalog did not contain (@2024.03) and standalone layers
// with no recipe (alphafold, pytorch, texlive, git), so examples/ had a 0% resolve
// rate and no test crossed a reference against the catalog — examples/*_test.go
// only parsed the YAML.
func TestShippedExamplesResolveOffline(t *testing.T) {
	clearAWSEnv(t)
	t.Setenv("STRATA_REGISTRY_URL", "")

	entries, err := os.ReadDir(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		count++
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.lock.yaml")
			cmd := newResolveCmd()
			cmd.SetArgs([]string{filepath.Join("..", "..", "examples", name), "-o", out})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("resolving example %s against the embedded catalog failed: %v", name, err)
			}
			if _, err := os.Stat(out); err != nil {
				t.Fatalf("%s: no lockfile written: %v", name, err)
			}
		})
	}
	if count == 0 {
		t.Fatal("no example profiles found; the path is wrong")
	}
}
