package fold

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// EjectToDir materializes all layers in lf into a plain directory tree at
// cfg.OutputDir. It does not require Strata at runtime after this operation.
//
// Output layout:
//
//	<OutputDir>/root/           — merged filesystem tree
//	<OutputDir>/env.sh          — bash snippet: PATH, LD_LIBRARY_PATH
//	<OutputDir>/environment     — systemd EnvironmentFile format
//	<OutputDir>/lock.yaml       — copy of the source lockfile
//	<OutputDir>/README          — usage instructions
//
// layerPaths must already be fetched and verified (e.g. via fetchLayersToCache).
func EjectToDir(ctx context.Context, lf *spec.LockFile, layerPaths []overlay.LayerPath, cfg EjectConfig) error {
	if cfg.OutputDir == "" {
		return fmt.Errorf("fold: eject: --eject output directory is required")
	}

	// Create output directory structure.
	rootDir := filepath.Join(cfg.OutputDir, "root")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return fmt.Errorf("fold: eject: creating output dir %q: %w", rootDir, err)
	}

	// Create temp workdir for overlay mount.
	workDir, err := os.MkdirTemp("", "strata-fold-eject-*")
	if err != nil {
		return fmt.Errorf("fold: eject: creating work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	// Mount overlay to get merged view.
	ovcfg := overlay.Config{
		LayersDir: filepath.Join(workDir, "layers"),
		RWDir:     filepath.Join(workDir, "rw"),
		MergedDir: filepath.Join(workDir, "env"),
	}
	ov, err := overlay.MountWithConfig(layerPaths, ovcfg)
	if err != nil {
		return fmt.Errorf("fold: eject: mounting overlay: %w", err)
	}
	defer ov.Cleanup() //nolint:errcheck

	// Recursively copy merged tree → <OutputDir>/root/.
	fmt.Fprintf(os.Stderr, "fold: eject: copying merged tree to %s\n", rootDir)
	if err := copyTree(ctx, ov.MergedPath, rootDir); err != nil {
		return fmt.Errorf("fold: eject: copying tree: %w", err)
	}

	// Write env.sh.
	envSh := buildEnvSh(lf, rootDir)
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "env.sh"), []byte(envSh), 0644); err != nil {
		return fmt.Errorf("fold: eject: writing env.sh: %w", err)
	}

	// Write environment (systemd EnvironmentFile).
	envFile := buildEnvironmentFile(lf, rootDir)
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "environment"), []byte(envFile), 0644); err != nil {
		return fmt.Errorf("fold: eject: writing environment: %w", err)
	}

	// Write lock.yaml (provenance).
	lockData, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("fold: eject: marshaling lockfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "lock.yaml"), lockData, 0644); err != nil {
		return fmt.Errorf("fold: eject: writing lock.yaml: %w", err)
	}

	// Write README.
	readme := buildReadme(lf, cfg.OutputDir)
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "README"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("fold: eject: writing README: %w", err)
	}

	fmt.Fprintf(os.Stderr, "fold: eject: done → %s\n", cfg.OutputDir)
	fmt.Fprintf(os.Stderr, "fold: eject: source: %s\n", filepath.Join(cfg.OutputDir, "env.sh"))
	return nil
}

// copyTree copies the directory tree at src into dst, preserving permissions,
// symlinks, and file content. Special files (devices, sockets, pipes) are skipped.
func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(destPath, info.Mode()&os.ModePerm)

		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Guard against absolute symlinks that escape the output root.
			// Relative symlinks are safe (they're relative to the symlink's own
			// location in the ejected tree). Absolute symlinks pointing outside
			// dst would follow host system paths, which is a security risk.
			if filepath.IsAbs(linkTarget) {
				clean := filepath.Clean(linkTarget)
				if !strings.HasPrefix(clean, dst) {
					log.Printf("fold: skipping absolute symlink %q -> %q (escapes output root)", rel, linkTarget)
					return nil
				}
			}
			_ = os.Remove(destPath)
			return os.Symlink(linkTarget, destPath)

		case info.Mode().IsRegular():
			return copyRegularFile(path, destPath, info.Mode()&os.ModePerm)

		default:
			// Skip devices, sockets, pipes, etc.
			return nil
		}
	})
}

// copyRegularFile copies a single regular file from src to dst with the given mode.
func copyRegularFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck
		return err
	}
	return out.Close()
}

// buildEnvSh constructs the bash env snippet for eject output.
func buildEnvSh(lf *spec.LockFile, rootDir string) string {
	pathParts, ldParts := buildEnvParts(lf, rootDir)

	var sb strings.Builder
	sb.WriteString("# Strata ejected environment — source this file before use\n")
	sb.WriteString("# Generated by strata fold --eject; Strata not required at runtime.\n")
	if len(pathParts) > 0 {
		fmt.Fprintf(&sb, "export PATH=%s:${PATH}\n", strings.Join(pathParts, ":"))
	}
	if len(ldParts) > 0 {
		fmt.Fprintf(&sb, "export LD_LIBRARY_PATH=%s:${LD_LIBRARY_PATH:-}\n", strings.Join(ldParts, ":"))
	}
	if lf.ProfileName != "" {
		fmt.Fprintf(&sb, "export STRATA_PROFILE=%s\n", shellQuote(lf.ProfileName))
	}
	return sb.String()
}

// buildEnvironmentFile constructs the systemd EnvironmentFile content.
func buildEnvironmentFile(lf *spec.LockFile, rootDir string) string {
	pathParts, ldParts := buildEnvParts(lf, rootDir)

	var sb strings.Builder
	if len(pathParts) > 0 {
		fmt.Fprintf(&sb, "PATH=%s\n", strings.Join(pathParts, ":"))
	}
	if len(ldParts) > 0 {
		fmt.Fprintf(&sb, "LD_LIBRARY_PATH=%s\n", strings.Join(ldParts, ":"))
	}
	if lf.ProfileName != "" {
		fmt.Fprintf(&sb, "STRATA_PROFILE=%s\n", lf.ProfileName)
	}
	return sb.String()
}

// buildEnvParts returns PATH and LD_LIBRARY_PATH component lists for the ejected tree.
func buildEnvParts(lf *spec.LockFile, rootDir string) (pathParts, ldParts []string) {
	lastVersionOf := make(map[string]string)
	for _, layer := range lf.Layers {
		if layer.InstallLayout != "flat" {
			lastVersionOf[layer.Name] = layer.Version
		}
	}
	for _, layer := range lf.Layers {
		if layer.InstallLayout == "flat" {
			continue
		}
		if lastVersionOf[layer.Name] != layer.Version {
			continue
		}
		base := fmt.Sprintf("%s/%s/%s", rootDir, layer.Name, layer.Version)
		pathParts = append(pathParts, base+"/bin")
		ldParts = append(ldParts, base+"/lib", base+"/lib64")
	}
	return pathParts, ldParts
}

// buildReadme returns a README string for the ejected directory.
func buildReadme(lf *spec.LockFile, outputDir string) string {
	var sb strings.Builder
	sb.WriteString("Strata Ejected Environment\n")
	sb.WriteString("==========================\n\n")
	if lf.ProfileName != "" {
		fmt.Fprintf(&sb, "Profile: %s\n\n", lf.ProfileName)
	}
	sb.WriteString("This directory was produced by 'strata fold --eject'. ")
	sb.WriteString("Strata is not required at runtime.\n\n")
	sb.WriteString("Directory layout:\n")
	fmt.Fprintf(&sb, "  %s/root/       — merged software tree\n", outputDir)
	fmt.Fprintf(&sb, "  %s/env.sh      — source this to set PATH and LD_LIBRARY_PATH\n", outputDir)
	fmt.Fprintf(&sb, "  %s/environment — systemd EnvironmentFile\n", outputDir)
	fmt.Fprintf(&sb, "  %s/lock.yaml   — source lockfile (provenance)\n", outputDir)
	sb.WriteString("\nUsage:\n")
	fmt.Fprintf(&sb, "  source %s/env.sh\n\n", outputDir)
	sb.WriteString("Layers included:\n")
	for _, layer := range lf.Layers {
		sha := layer.SHA256
		if len(sha) > 16 {
			sha = sha[:16]
		}
		fmt.Fprintf(&sb, "  %s@%s (sha256:%s)\n", layer.Name, layer.Version, sha)
	}
	return sb.String()
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
