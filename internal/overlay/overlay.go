// Package overlay assembles OverlayFS environments from squashfs layers.
//
// The assembly sequence is:
//  1. Mount each layer as a read-only squashfs loopback device.
//  2. Mount a tmpfs for the ephemeral upper and work directories.
//  3. Assemble the OverlayFS at /strata/env.
//  4. Write environment config files so processes see the merged view.
//
// Mount and Cleanup are Linux-only (syscall.Mount). On other platforms they
// return ErrNotSupported. ConfigureEnvironment is platform-neutral.
package overlay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// ErrNotSupported is returned on non-Linux platforms where OverlayFS
// assembly is not available.
var ErrNotSupported = errors.New("overlay: OverlayFS assembly requires Linux")

// LayerPath is a pulled squashfs layer ready to be mounted.
type LayerPath struct {
	ID         string // layer ID, used for mount point naming
	SHA256     string // hex SHA256, used as cache key
	Path       string // local .sqfs file path
	MountOrder int    // from lockfile; 1 = bottom of stack
}

// Overlay is a mounted OverlayFS assembly. Call Cleanup when done.
// The overlay stays mounted until Cleanup is called (i.e., until the
// strata-agent process exits).
type Overlay struct {
	MergedPath string // /strata/env — what processes see
	UpperDir   string // tmpfs subdir for ephemeral writes
	WorkDir    string // tmpfs subdir for OverlayFS internal use

	// rwMount is the single tmpfs mount point that contains both UpperDir
	// and WorkDir as subdirectories. OverlayFS requires upper and work to
	// be on the same filesystem; a single tmpfs satisfies this constraint.
	rwMount string //nolint:unused // used in mount_linux.go

	// squashMountPoints holds the squashfs mount points in MountOrder
	// (ascending). Cleanup unmounts them in reverse order.
	squashMountPoints []string //nolint:unused // used in mount_linux.go
}

// Mount and Cleanup are declared here; implemented in mount_linux.go (Linux)
// and mount_stub.go (all other platforms).

// ConfigureEnvironment writes the three environment files after a successful
// mount. rootDir is the filesystem root prefix; use "/" in production and
// t.TempDir() in tests.
//
// Files written:
//   - <rootDir>/etc/profile.d/strata.sh     — shell env (PATH, LD_LIBRARY_PATH, STRATA_*)
//   - <rootDir>/etc/strata/environment      — systemd EnvironmentFile (KEY=VALUE)
//   - <rootDir>/etc/strata/active.lock.yaml — the active lockfile
func ConfigureEnvironment(lockfile *spec.LockFile, ov *Overlay, rootDir string) error {
	mergedPath := "/strata/env"
	if ov != nil && ov.MergedPath != "" {
		mergedPath = ov.MergedPath
	}

	// --- /etc/profile.d/strata.sh ---
	profileD := filepath.Join(rootDir, "etc", "profile.d")
	if err := os.MkdirAll(profileD, 0755); err != nil {
		return fmt.Errorf("overlay: creating profile.d: %w", err)
	}

	var sh strings.Builder
	sh.WriteString("# Strata environment — auto-generated, do not edit\n")
	fmt.Fprintf(&sh, "export PATH=%s/usr/local/bin:%s/usr/bin:${PATH}\n", mergedPath, mergedPath)
	fmt.Fprintf(&sh, "export LD_LIBRARY_PATH=%s/usr/lib:%s/usr/lib64:${LD_LIBRARY_PATH}\n", mergedPath, mergedPath)
	fmt.Fprintf(&sh, "export STRATA_PROFILE=%s\n", shellQuote(lockfile.ProfileName))
	fmt.Fprintf(&sh, "export STRATA_REKOR_ENTRY=%s\n", shellQuote(lockfile.RekorEntry))
	for k, v := range lockfile.Env {
		fmt.Fprintf(&sh, "export %s=%s\n", k, shellQuote(v))
	}
	if err := os.WriteFile(filepath.Join(profileD, "strata.sh"), []byte(sh.String()), 0644); err != nil {
		return fmt.Errorf("overlay: writing strata.sh: %w", err)
	}

	// --- /etc/strata/environment (systemd EnvironmentFile) ---
	strataDir := filepath.Join(rootDir, "etc", "strata")
	if err := os.MkdirAll(strataDir, 0755); err != nil {
		return fmt.Errorf("overlay: creating /etc/strata: %w", err)
	}

	var env strings.Builder
	fmt.Fprintf(&env, "PATH=%s/usr/local/bin:%s/usr/bin\n", mergedPath, mergedPath)
	fmt.Fprintf(&env, "LD_LIBRARY_PATH=%s/usr/lib:%s/usr/lib64\n", mergedPath, mergedPath)
	fmt.Fprintf(&env, "STRATA_PROFILE=%s\n", lockfile.ProfileName)
	fmt.Fprintf(&env, "STRATA_REKOR_ENTRY=%s\n", lockfile.RekorEntry)
	for k, v := range lockfile.Env {
		fmt.Fprintf(&env, "%s=%s\n", k, v)
	}
	if err := os.WriteFile(filepath.Join(strataDir, "environment"), []byte(env.String()), 0644); err != nil {
		return fmt.Errorf("overlay: writing environment: %w", err)
	}

	// --- /etc/strata/active.lock.yaml ---
	lockData, err := yaml.Marshal(lockfile)
	if err != nil {
		return fmt.Errorf("overlay: marshaling lockfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(strataDir, "active.lock.yaml"), lockData, 0644); err != nil {
		return fmt.Errorf("overlay: writing active.lock.yaml: %w", err)
	}

	return nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
