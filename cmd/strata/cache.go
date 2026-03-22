package main

import (
	"os"
	"path/filepath"
)

// defaultCacheDir returns the layer cache directory appropriate for the
// current process's privilege level, following XDG Base Dir Spec.
//
//   - Root: /strata/cache (production system path)
//   - Non-root: $XDG_CACHE_HOME/strata/layers, or ~/.cache/strata/layers
func defaultCacheDir() string {
	if os.Getuid() == 0 {
		return "/strata/cache"
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "strata", "layers")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "strata", "layers")
	}
	return filepath.Join(home, ".cache", "strata", "layers")
}
