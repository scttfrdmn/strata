package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VerifyLayerLayout checks that a mounted layer's on-disk structure matches the
// Name/Version/InstallLayout its manifest claims.
//
// Those three fields are read on the assembly path to build PATH and
// LD_LIBRARY_PATH, but are covered by no signature — the cosign bundle attests
// the layer bytes, not the manifest document (#146). Under an A1 registry write
// an attacker edits them in a served manifest, breaks no signature, and
// redirects which binaries a login shell resolves. Deriving the layout from the
// mounted bytes turns those unsigned assertions into checkable ones, so a
// tampered manifest is refused rather than silently degrading the environment to
// host binaries.
//
// layerRoot is the layer's own mount point — a single layer, not the merged
// overlay, so the check is attributed to the layer whose manifest is under test:
//
//   - A versioned layer (InstallLayout != "flat") must contain <name>/<version>/bin,
//     the exact directory ConfigureEnvironment puts on PATH. Its absence means the
//     manifest names a version the bytes do not provide (e.g. a served manifest
//     with version bumped 3.11.9 → 3.11.10).
//   - A "flat" layer must NOT contain <name>/<version>/bin. Its presence means a
//     versioned layer was relabeled "flat" to drop it from PATH while it still
//     mounts — a silent downgrade to host binaries.
func VerifyLayerLayout(layerRoot, name, version, installLayout string) error {
	// name and version are joined into a path below; a value with a separator or
	// traversal element is itself the tampering and is refused before the stat.
	for _, f := range []struct{ label, val string }{{"name", name}, {"version", version}} {
		if f.val == "" || f.val == "." || f.val == ".." ||
			strings.ContainsAny(f.val, `/\`) || strings.ContainsRune(f.val, 0) {
			return fmt.Errorf("overlay: layer manifest %s %q is not a safe path component (#146)", f.label, f.val)
		}
	}

	versionedBin := filepath.Join(layerRoot, name, version, "bin")
	present := isDir(versionedBin)

	if installLayout == "flat" {
		if present {
			return fmt.Errorf("overlay: layer %s/%s claims install_layout=flat but contains %s/%s/bin — a versioned layer cannot be flat; the manifest may be tampered (#146)",
				name, version, name, version)
		}
		return nil
	}
	if !present {
		return fmt.Errorf("overlay: layer %s/%s declares a versioned layout but has no %s/%s/bin — Name/Version do not match the layer contents; the manifest may be tampered (#146)",
			name, version, name, version)
	}
	return nil
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
