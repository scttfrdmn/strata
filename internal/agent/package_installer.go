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
			err = installPip(ctx, ps.Packages, pathEnv)
		case spec.PackageManagerConda:
			err = installConda(ctx, ps.Packages, ps.Env, pathEnv)
		case spec.PackageManagerCRAN:
			err = installCRAN(ctx, ps.Packages, pathEnv)
		default:
			return fmt.Errorf("package installer: unsupported manager %q", ps.Manager)
		}
		if err != nil {
			return fmt.Errorf("package installer: %s: %w", ps.Manager, err)
		}
	}
	return nil
}

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

func installPip(ctx context.Context, entries []spec.ResolvedPackageEntry, pathEnv string) error {
	for _, e := range entries {
		pkg := e.Name + "==" + e.Version
		if err := runCmd(ctx, pathEnv, "pip", "install", "--quiet", pkg); err != nil {
			return fmt.Errorf("pip install %q: %w", e.Name, err)
		}
	}
	return nil
}

func installConda(ctx context.Context, entries []spec.ResolvedPackageEntry, env string, pathEnv string) error {
	args := []string{"install", "-y", "--quiet"}
	if env != "" {
		args = append(args, "-n", env)
	}
	for _, e := range entries {
		pkg := e.Name
		if e.Version != "latest" && e.Version != "" {
			pkg = e.Name + "=" + e.Version
		}
		if err := runCmd(ctx, pathEnv, "conda", append(args, pkg)...); err != nil {
			return fmt.Errorf("conda install %q: %w", e.Name, err)
		}
	}
	return nil
}

func installCRAN(ctx context.Context, entries []spec.ResolvedPackageEntry, pathEnv string) error {
	for _, e := range entries {
		if !cranNameRe.MatchString(e.Name) {
			return fmt.Errorf("invalid CRAN package name %q", e.Name)
		}
		script := fmt.Sprintf(
			`install.packages("%s", repos="https://cran.r-project.org", quiet=TRUE)`,
			e.Name,
		)
		if err := runCmd(ctx, pathEnv, "Rscript", "-e", script); err != nil {
			return fmt.Errorf("Rscript install %q: %w", e.Name, err)
		}
	}
	return nil
}
