//go:build linux

package overlay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// fuseMountStrategy implements MountStrategy using FUSE userspace tools:
//   - squashfuse mounts squashfs images without root
//   - fuse-overlayfs assembles the OverlayFS without CAP_SYS_ADMIN
//
// Required binaries: squashfuse, fuse-overlayfs, and fusermount3 or fusermount.
type fuseMountStrategy struct{}

// MountSquashfs mounts sqfsPath at mountPoint using squashfuse.
func (fuseMountStrategy) MountSquashfs(sqfsPath, mountPoint string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("squashfuse", sqfsPath, mountPoint)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("squashfuse: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

// MountTmpfs creates the directory at mountPoint (FUSE overlay does not need
// a real tmpfs; a regular directory is sufficient for the upper and work dirs).
func (fuseMountStrategy) MountTmpfs(mountPoint string) error {
	return os.MkdirAll(mountPoint, 0755)
}

// MountOverlay assembles an OverlayFS using fuse-overlayfs.
// lowerDirs are passed in ascending MountOrder; they are reversed internally
// so the highest-priority layer is leftmost in the lowerdir option.
func (fuseMountStrategy) MountOverlay(lowerDirs []string, upper, work, merged string) error {
	// fuse-overlayfs searches lowerdir left-to-right (same as kernel overlay).
	// Reverse so highest MountOrder is first.
	reversed := make([]string, len(lowerDirs))
	copy(reversed, lowerDirs)
	reverseStrings(reversed)

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(reversed, ":"), upper, work)

	var stderr bytes.Buffer
	cmd := exec.Command("fuse-overlayfs", "-o", opts, merged)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fuse-overlayfs: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

// Unmount unmounts a FUSE mount point using fusermount3 (preferred) or fusermount.
func (fuseMountStrategy) Unmount(mountPoint string, _ bool) error {
	if err := exec.Command("fusermount3", "-u", mountPoint).Run(); err == nil {
		return nil
	}
	return exec.Command("fusermount", "-u", mountPoint).Run()
}
