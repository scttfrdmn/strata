// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scttfrdmn/strata/spec"
)

// cranNameRe matches valid CRAN package names: letters, digits, dots, underscores, hyphens.
// R package names must start with a letter, but we validate only the character set here
// to prevent shell/R script injection.
var cranNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ExecPackageInstaller installs resolved package sets by executing pip,
// conda, and Rscript from the overlay's merged bin directories.
type ExecPackageInstaller struct{}

// Install installs all package sets in pkgs into the overlay at mergedPath.
// It builds a PATH from the merged bin directories and dispatches to the
// appropriate installer for each package manager.
func (ExecPackageInstaller) Install(ctx context.Context, pkgs []spec.ResolvedPackageSet, mergedPath string) error {
	// Build PATH by scanning for bin/ directories under the merged root.
	// Entries two levels deep (mergedPath/<name>/<version>/bin) cover
	// the versioned install layout used by all non-flat Strata layers.
	binDirs := collectBinDirs(mergedPath)
	pathEnv := strings.Join(binDirs, ":") + ":" + os.Getenv("PATH")

	for _, ps := range pkgs {
		var err error
		switch ps.Manager {
		case spec.PackageManagerPip:
			err = installPip(ctx, ps.Packages, pathEnv, runCmd)
		case spec.PackageManagerConda:
			err = installConda(ctx, ps.Packages, ps.Env, pathEnv, runCmd)
		case spec.PackageManagerCRAN:
			err = installCRAN(ctx, ps.Packages, pathEnv, runCmd)
		default:
			return fmt.Errorf("package installer: unsupported manager %q", ps.Manager)
		}
		if err != nil {
			return fmt.Errorf("package installer: %s: %w", ps.Manager, err)
		}
	}
	return nil
}

// cmdRunner runs an installer command. It is a seam: Install passes runCmd,
// which shells out; tests pass a fake that records the arguments, so the exact
// pip/conda invocation — the pin and hash flags that make an install
// reproducible — is asserted without pip or conda on PATH.
type cmdRunner func(ctx context.Context, pathEnv, name string, args ...string) error

// collectBinDirs returns all bin/ directories two levels under mergedPath.
func collectBinDirs(mergedPath string) []string {
	var dirs []string
	// Walk the immediate children: mergedPath/<name>/
	entries, err := os.ReadDir(mergedPath)
	if err != nil {
		return dirs
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nameDir := filepath.Join(mergedPath, e.Name())
		// Walk the version subdirectories: mergedPath/<name>/<version>/
		versions, err := os.ReadDir(nameDir)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			binDir := filepath.Join(nameDir, v.Name(), "bin")
			if info, err := os.Stat(binDir); err == nil && info.IsDir() {
				dirs = append(dirs, binDir)
			}
		}
	}
	return dirs
}

func runCmd(ctx context.Context, pathEnv string, name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PATH="+pathEnv)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %q: %w\n%s", name, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

func installPip(ctx context.Context, entries []spec.ResolvedPackageEntry, pathEnv string, run cmdRunner) error {
	for _, e := range entries {
		if e.Version == "" {
			return fmt.Errorf("pip package %q has no pinned version — refusing to install a floating package (the environment would not be reproducible)", e.Name)
		}
		req := e.Name + "==" + e.Version
		args := []string{"install", "--quiet"}
		if e.SHA256 != "" {
			// Enforce the recorded wheel digest. --hash makes pip verify the
			// downloaded artifact against it (and implies --require-hashes);
			// --no-deps keeps the check to this exact artifact, so a dependency
			// that has no recorded hash does not make --require-hashes fail —
			// each dependency the environment needs is its own recorded entry.
			args = append(args, "--no-deps", "--require-hashes", req, "--hash=sha256:"+e.SHA256)
		} else {
			// No recorded hash: the version is pinned but the bytes are not, so
			// this install is not byte-reproducible. #139 surfaces that; here we
			// at least do not let the version float.
			args = append(args, req)
		}
		if err := run(ctx, pathEnv, "pip", args...); err != nil {
			return fmt.Errorf("pip install %q: %w", e.Name, err)
		}
	}
	return nil
}

func installConda(ctx context.Context, entries []spec.ResolvedPackageEntry, env string, pathEnv string, run cmdRunner) error {
	base := []string{"install", "-y", "--quiet"}
	if env != "" {
		base = append(base, "-n", env)
	}
	for _, e := range entries {
		if e.Version == "latest" || e.Version == "" {
			return fmt.Errorf("conda package %q has version %q — refusing to install a floating version; pin an exact version so the environment is reproducible", e.Name, e.Version)
		}
		// Fresh args per package so the shared base slice is never aliased.
		args := append(append([]string{}, base...), e.Name+"="+e.Version)
		if err := run(ctx, pathEnv, "conda", args...); err != nil {
			return fmt.Errorf("conda install %q: %w", e.Name, err)
		}
	}
	return nil
}

func installCRAN(ctx context.Context, entries []spec.ResolvedPackageEntry, pathEnv string, run cmdRunner) error {
	for _, e := range entries {
		if !cranNameRe.MatchString(e.Name) {
			return fmt.Errorf("invalid CRAN package name %q", e.Name)
		}
		script := fmt.Sprintf(
			`install.packages("%s", repos="https://cran.r-project.org", quiet=TRUE)`,
			e.Name,
		)
		if err := run(ctx, pathEnv, "Rscript", "-e", script); err != nil {
			return fmt.Errorf("rscript install %q: %w", e.Name, err)
		}
	}
	return nil
}
