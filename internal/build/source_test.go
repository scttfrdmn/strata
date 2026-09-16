// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// sourceServer serves body at /artifact and records how many times it was hit.
func sourceServer(t *testing.T, body []byte) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/artifact" {
			http.NotFound(w, r)
			return
		}
		w.Write(body) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestStageSources_VerifiesAndStages: a source whose bytes match its pinned
// digest is fetched and staged under its filename, byte-for-byte.
func TestStageSources_VerifiesAndStages(t *testing.T) {
	body := []byte("pretend source tarball contents")
	srv, _ := sourceServer(t, body)
	dir := t.TempDir()

	src := RecipeSource{URL: srv.URL + "/artifact", SHA256: sha256Hex(body), File: "pkg.tar"}
	if err := StageSources(context.Background(), []RecipeSource{src}, dir, "", srv.Client()); err != nil {
		t.Fatalf("StageSources: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pkg.tar"))
	if err != nil {
		t.Fatalf("staged file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("staged bytes differ from source")
	}
}

// TestStageSources_RejectsDigestMismatch is the point of the field: bytes that
// do not match the pin fail the build, with both digests, and nothing is staged.
func TestStageSources_RejectsDigestMismatch(t *testing.T) {
	served := []byte("what the registry actually serves today")
	srv, _ := sourceServer(t, served)
	dir := t.TempDir()

	// Pin a *different* content's digest — the drift/tamper case.
	src := RecipeSource{URL: srv.URL + "/artifact", SHA256: sha256Hex([]byte("what the recipe author pinned")), File: "pkg.tar"}
	err := StageSources(context.Background(), []RecipeSource{src}, dir, "", srv.Client())
	if err == nil {
		t.Fatal("StageSources accepted bytes that do not match the pinned digest")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("error does not name the mismatch: %v", err)
	}
	// Both the pinned and the actual digest must be in the message so the failure
	// is actionable.
	if !strings.Contains(err.Error(), sha256Hex(served)) || !strings.Contains(err.Error(), src.SHA256) {
		t.Errorf("error omits the expected and/or actual digest: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a mismatched source was left in the sources dir: %v", entries)
	}
}

// TestStageSources_RejectsFetchError: a source that cannot be fetched (not 200)
// fails rather than staging an empty or partial file.
func TestStageSources_RejectsFetchError(t *testing.T) {
	srv, _ := sourceServer(t, []byte("x"))
	dir := t.TempDir()
	src := RecipeSource{URL: srv.URL + "/missing", SHA256: sha256Hex([]byte("x")), File: "pkg.tar"}
	err := StageSources(context.Background(), []RecipeSource{src}, dir, "", srv.Client())
	if err == nil {
		t.Fatal("StageSources accepted a non-200 fetch")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error does not name the status: %v", err)
	}
}

// TestStagedFile_DefaultsToURLBasename: the staged name defaults to the URL's
// last path element when File is unset.
func TestStagedFile_DefaultsToURLBasename(t *testing.T) {
	s := RecipeSource{URL: "https://example.com/a/b/bcftools-1.21.tar.bz2"}
	if got := s.StagedFile(); got != "bcftools-1.21.tar.bz2" {
		t.Errorf("StagedFile() = %q, want bcftools-1.21.tar.bz2", got)
	}
	if got := (RecipeSource{URL: "https://x/y", File: "custom.tar"}).StagedFile(); got != "custom.tar" {
		t.Errorf("explicit File not honoured: %q", got)
	}
}

// TestStageSourcesForBuild covers the pipeline-side helper that Run uses: no
// sources yields no directory, declared sources are staged into a fresh temp
// dir, and a mismatch fails without leaking the directory.
func TestStageSourcesForBuild(t *testing.T) {
	t.Run("no sources returns empty dir", func(t *testing.T) {
		dir, err := stageSourcesForBuild(context.Background(), nil, "x86_64")
		if err != nil || dir != "" {
			t.Fatalf("stageSourcesForBuild(nil) = %q, %v; want \"\", nil", dir, err)
		}
	})

	t.Run("declared source is staged", func(t *testing.T) {
		body := []byte("tarball")
		srv, _ := sourceServer(t, body)
		// stageSourcesForBuild uses the default HTTP client; an httptest server's
		// loopback URL is reachable by it.
		dir, err := stageSourcesForBuild(context.Background(),
			[]RecipeSource{{URL: srv.URL + "/artifact", SHA256: sha256Hex(body), File: "t.tar"}}, "x86_64")
		if err != nil {
			t.Fatalf("stageSourcesForBuild: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		if dir == "" {
			t.Fatal("expected a staging dir")
		}
		if _, statErr := os.Stat(filepath.Join(dir, "t.tar")); statErr != nil {
			t.Errorf("source not staged: %v", statErr)
		}
	})

	t.Run("mismatch fails and leaves no dir", func(t *testing.T) {
		body := []byte("served")
		srv, _ := sourceServer(t, body)
		dir, err := stageSourcesForBuild(context.Background(),
			[]RecipeSource{{URL: srv.URL + "/artifact", SHA256: sha256Hex([]byte("pinned")), File: "t.tar"}}, "x86_64")
		if err == nil {
			t.Fatal("expected a digest-mismatch error")
		}
		if dir != "" {
			_ = os.RemoveAll(dir)
			t.Errorf("a staging dir was returned alongside an error: %q", dir)
		}
	})
}

// TestWithSourcesEnv: STRATA_SOURCES is appended only when a dir was staged.
func TestWithSourcesEnv(t *testing.T) {
	base := []string{"A=1"}
	if got := withSourcesEnv(base, ""); len(got) != 1 {
		t.Errorf("withSourcesEnv(_, \"\") appended an entry: %v", got)
	}
	got := withSourcesEnv(base, "/tmp/src")
	if len(got) != 2 || got[1] != "STRATA_SOURCES=/tmp/src" {
		t.Errorf("withSourcesEnv did not append STRATA_SOURCES: %v", got)
	}
}

// TestStageSources_ArchFiltering: a build stages the arch-agnostic sources plus
// the one matching its target arch, and skips sources pinned for another arch —
// so an arch-specific binary distribution stages exactly the right artifact.
func TestStageSources_ArchFiltering(t *testing.T) {
	agnostic := []byte("agnostic tarball")
	x86 := []byte("x86_64 binary")
	arm := []byte("arm64 binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/common":
			w.Write(agnostic) //nolint:errcheck
		case "/x86":
			w.Write(x86) //nolint:errcheck
		case "/arm":
			w.Write(arm) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	sources := []RecipeSource{
		{URL: srv.URL + "/common", SHA256: sha256Hex(agnostic), File: "common.tar"},
		{URL: srv.URL + "/x86", SHA256: sha256Hex(x86), File: "bin.tar", Arch: "x86_64"},
		{URL: srv.URL + "/arm", SHA256: sha256Hex(arm), File: "bin.tar", Arch: "arm64"},
	}

	dir := t.TempDir()
	if err := StageSources(context.Background(), sources, dir, "x86_64", srv.Client()); err != nil {
		t.Fatalf("StageSources(x86_64): %v", err)
	}
	// common.tar (agnostic) + bin.tar (x86_64) staged; the arm64 source skipped.
	if got, _ := os.ReadFile(filepath.Join(dir, "bin.tar")); string(got) != string(x86) {
		t.Errorf("bin.tar is not the x86_64 artifact: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "common.tar")); err != nil {
		t.Errorf("arch-agnostic source not staged: %v", err)
	}
}

// TestRecipeMeta_Validate_Sources drives the schema-level guard: a declared
// source must be genuinely pinned (URL + well-formed sha256) and stage to a safe
// plain filename, so a recipe cannot claim a source it does not pin.
func TestRecipeMeta_Validate_Sources(t *testing.T) {
	base := func(srcs []RecipeSource) *RecipeMeta {
		return &RecipeMeta{
			Name: "demo", Version: "1.0", Tier: "application", ABI: "linux-gnu-2.34",
			Provides: []spec.Capability{{Name: "demo", Version: "1.0"}},
			Sources:  srcs,
		}
	}
	good := sha256Hex([]byte("x"))

	cases := []struct {
		name    string
		srcs    []RecipeSource
		wantErr string // "" = must pass
	}{
		{"valid pinned source", []RecipeSource{{URL: "https://x/a.tar", SHA256: good}}, ""},
		{"no sources is allowed", nil, ""},
		{"empty url", []RecipeSource{{URL: "", SHA256: good}}, "empty url"},
		{"missing digest", []RecipeSource{{URL: "https://x/a.tar", SHA256: ""}}, "sha256"},
		{"malformed digest", []RecipeSource{{URL: "https://x/a.tar", SHA256: "not-a-digest"}}, "sha256"},
		{"path in file", []RecipeSource{{URL: "https://x/a.tar", SHA256: good, File: "../evil"}}, "plain filename"},
		{"duplicate staged name (same arch)", []RecipeSource{
			{URL: "https://x/a.tar", SHA256: good, File: "same.tar"},
			{URL: "https://y/b.tar", SHA256: good, File: "same.tar"},
		}, "stages two sources"},
		{"same file across arches is allowed", []RecipeSource{
			{URL: "https://x/x86.tar", SHA256: good, File: "bin.tar", Arch: "x86_64"},
			{URL: "https://x/arm.tar", SHA256: good, File: "bin.tar", Arch: "arm64"},
		}, ""},
		{"unsupported arch", []RecipeSource{
			{URL: "https://x/a.tar", SHA256: good, Arch: "ppc64le"},
		}, "unsupported arch"},
		{"agnostic collides with arch-specific", []RecipeSource{
			{URL: "https://x/a.tar", SHA256: good, File: "bin.tar"},
			{URL: "https://x/x86.tar", SHA256: good, File: "bin.tar", Arch: "x86_64"},
		}, "stages two sources"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := base(tc.srcs).Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}
