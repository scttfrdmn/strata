package spec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateLayerID reports whether id is safe to use as a single filesystem path
// component.
//
// A layer ID names a mount point, a cached squashfs file, and an OCI unpack
// directory — each built with filepath.Join, which calls Clean and therefore
// *resolves* ".." rather than rejecting it. So an id like "../../etc/cron.d/x"
// joined to a base directory escapes that directory, and at the mount site the
// escape drives os.MkdirAll — a write primitive. Every site that builds a path
// from a layer ID must call this first; the comment that once claimed
// filepath.Join "prevents escape" was exactly backwards.
//
// A valid id is a non-empty single path element: no path separator, not "." or
// "..", no NUL byte, and local by filepath.IsLocal. Layer IDs are
// machine-generated in the form name-version-abi-arch (e.g.
// "python-3.13.2-linux-gnu-2.34-x86_64"), so this rejects only ids that could
// not have come from the resolver — an escaping id is a tampered or
// hand-written lockfile, which is precisely the case that must not reach a
// filesystem operation.
func ValidateLayerID(id string) error {
	if id == "" {
		return fmt.Errorf("spec: layer id is empty")
	}
	if strings.ContainsRune(id, 0) {
		return fmt.Errorf("spec: layer id %q contains a NUL byte", id)
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("spec: layer id %q contains a path separator; a layer id must be a single path component", id)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("spec: layer id %q is a path-traversal element", id)
	}
	// Belt and suspenders: reject anything filepath.IsLocal would reject
	// (absolute paths, "..", reserved names) even if the checks above missed a
	// platform-specific form.
	if !filepath.IsLocal(id) {
		return fmt.Errorf("spec: layer id %q is not a safe local path component", id)
	}
	return nil
}
