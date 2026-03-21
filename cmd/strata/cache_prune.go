package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

func newCachePruneCmd() *cobra.Command {
	var cacheDir, lockfilePath string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "cache-prune",
		Short: "Remove cached layer files not referenced by the active lockfile",
		Long: `Delete .sqfs files in the local layer cache that are not referenced
by the active lockfile. Safe and reversible — layers are re-downloaded
from the registry on next boot.

Intended for use on running Strata instances to free disk space.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCachePrune(cacheDir, lockfilePath, dryRun)
		},
	}

	cmd.Flags().StringVar(&cacheDir, "cache-dir", "/strata/cache",
		"directory containing cached .sqfs layer files")
	cmd.Flags().StringVar(&lockfilePath, "lockfile", "/etc/strata/active.lock.yaml",
		"path to the active lockfile")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list candidates without deleting")
	return cmd
}

func runCachePrune(cacheDir, lockfilePath string, dryRun bool) error {
	// Read the active lockfile.
	data, err := os.ReadFile(lockfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no active lockfile at %s — not running on a Strata instance?", lockfilePath)
		}
		return fmt.Errorf("reading lockfile: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return fmt.Errorf("parsing lockfile: %w", err)
	}

	// Build the set of in-use SHA256 hashes.
	inUse := make(map[string]bool, len(lf.Layers))
	for _, layer := range lf.Layers {
		if layer.SHA256 != "" {
			inUse[layer.SHA256] = true
		}
	}

	// List all .sqfs files in the cache directory.
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("cache directory %s does not exist — nothing to prune\n", cacheDir)
			return nil
		}
		return fmt.Errorf("reading cache dir: %w", err)
	}

	// Find candidates for deletion.
	type candidate struct {
		path string
		size int64
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sqfs") {
			continue
		}
		sha256 := strings.TrimSuffix(entry.Name(), ".sqfs")
		if inUse[sha256] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path: filepath.Join(cacheDir, entry.Name()),
			size: info.Size(),
		})
	}

	if len(candidates) == 0 {
		fmt.Println("nothing to prune — all cached layers are in use")
		return nil
	}

	// Report candidates.
	var totalBytes int64
	for _, c := range candidates {
		totalBytes += c.size
		fmt.Printf("  %s (%s)\n", filepath.Base(c.path), formatBytes(c.size))
	}

	if dryRun {
		fmt.Printf("\n%d file(s) would be removed (%s freed)\n", len(candidates), formatBytes(totalBytes))
		return nil
	}

	// Delete candidates.
	var removed int
	for _, c := range candidates {
		if err := os.Remove(c.path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing %s: %v\n", c.path, err)
			continue
		}
		removed++
	}

	fmt.Printf("\nremoved %d file(s) (%s freed)\n", removed, formatBytes(totalBytes))
	return nil
}
