// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// StageSources fetches every declared source into destDir, verifying each
// against its pinned SHA256 before the build runs. A mismatch — a source that
// drifted or was tampered with — fails with both the pinned and the actual
// digest, so the build never proceeds from bytes the recipe did not pin (#68).
// The verified files are what build.sh reads from $STRATA_SOURCES; it fetches
// nothing itself, which is what makes the layer reproducible from the recipe.
//
// client is a parameter so tests drive it against an httptest server; the build
// pipeline passes nil for a default client with a generous timeout.
func StageSources(ctx context.Context, sources []RecipeSource, destDir string, client *http.Client) error {
	if len(sources) == 0 {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("build: creating sources dir: %w", err)
	}
	for _, s := range sources {
		if err := fetchAndVerify(ctx, s, destDir, client); err != nil {
			return err
		}
	}
	return nil
}

// stageSourcesForBuild creates a temp directory, stages every declared source
// into it (fetching and verifying each), and returns the directory the build
// pipeline exposes as $STRATA_SOURCES. It returns "" and no error when the recipe
// declares no sources. On any staging failure it removes the directory, so the
// caller never sees a partially-populated sources dir.
func stageSourcesForBuild(ctx context.Context, sources []RecipeSource) (string, error) {
	if len(sources) == 0 {
		return "", nil
	}
	dir, err := os.MkdirTemp("", "strata-sources-*")
	if err != nil {
		return "", fmt.Errorf("build: creating sources dir: %w", err)
	}
	if err := StageSources(ctx, sources, dir, nil); err != nil {
		os.RemoveAll(dir) //nolint:errcheck
		return "", err
	}
	return dir, nil
}

// withSourcesEnv appends STRATA_SOURCES=dir to env when dir is non-empty, and
// returns env unchanged otherwise. It exists so the caller stays branch-free.
func withSourcesEnv(env []string, dir string) []string {
	if dir == "" {
		return env
	}
	return append(env, "STRATA_SOURCES="+dir)
}

// fetchAndVerify downloads one source, hashes it while streaming, and stages it
// only if the digest matches. It writes to a temp file first and renames on
// success, so a failed or mismatched fetch never leaves a partial file that a
// build could mistake for a verified source.
func fetchAndVerify(ctx context.Context, s RecipeSource, destDir string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return fmt.Errorf("build: building request for source %s: %w", s.URL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("build: fetching source %s: %w", s.URL, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("build: fetching source %s: unexpected status %d", s.URL, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(destDir, ".src-*")
	if err != nil {
		return fmt.Errorf("build: staging source %s: %w", s.URL, err)
	}
	tmpPath := tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("build: reading source %s: %w", s.URL, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("build: staging source %s: %w", s.URL, err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != s.SHA256 {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("build: source %s digest mismatch: recipe pins sha256:%s but the fetched bytes are sha256:%s — refusing to build from an unpinned or tampered source", s.URL, s.SHA256, got)
	}

	dest := filepath.Join(destDir, s.StagedFile())
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("build: staging source %s: %w", s.URL, err)
	}
	return nil
}
