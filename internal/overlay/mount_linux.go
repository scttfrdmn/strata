//go:build linux

package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	strataLayersDir = "/strata/layers"
	strataUpperDir  = "/strata/upper"
	strataWorkDir   = "/strata/work"
	strataMergedDir = "/strata/env"
)

// Mount mounts each layer as a read-only squashfs loopback device, creates a
// tmpfs for the upper and work directories, then assembles the OverlayFS at
// /strata/env.
//
// Layers are mounted in ascending MountOrder (MountOrder=1 is at the bottom
// of the OverlayFS lower stack). The highest-MountOrder layer wins in the
// OverlayFS lookup order.
//
// If any mount step fails, all previously mounted filesystems are unmounted
// before returning the error. No dangling mounts are left behind.
func Mount(layers []LayerPath) (*Overlay, error) {
	// Sort ascending by MountOrder so we mount bottom-of-stack first.
	sorted := make([]LayerPath, len(layers))
	copy(sorted, layers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MountOrder < sorted[j].MountOrder
	})

	// Mount each layer as squashfs.
	var squashPoints []string
	for _, layer := range sorted {
		mp := filepath.Join(strataLayersDir, layer.ID)
		if err := os.MkdirAll(mp, 0755); err != nil {
			cleanupSquash(squashPoints)
			return nil, fmt.Errorf("overlay: creating mount point for %q: %w", layer.ID, err)
		}
		if err := syscall.Mount(layer.Path, mp, "squashfs", syscall.MS_RDONLY, "loop"); err != nil {
			cleanupSquash(squashPoints)
			return nil, fmt.Errorf("overlay: mounting squashfs %q at %q: %w", layer.ID, mp, err)
		}
		squashPoints = append(squashPoints, mp)
	}

	// tmpfs for upper (ephemeral writes) and work (OverlayFS internal).
	if err := os.MkdirAll(strataUpperDir, 0755); err != nil {
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: creating upper dir: %w", err)
	}
	if err := syscall.Mount("tmpfs", strataUpperDir, "tmpfs", 0, "size=1g,mode=755"); err != nil {
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: mounting upper tmpfs: %w", err)
	}

	if err := os.MkdirAll(strataWorkDir, 0755); err != nil {
		_ = syscall.Unmount(strataUpperDir, syscall.MNT_DETACH)
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: creating work dir: %w", err)
	}
	if err := syscall.Mount("tmpfs", strataWorkDir, "tmpfs", 0, "size=100m,mode=755"); err != nil {
		_ = syscall.Unmount(strataUpperDir, syscall.MNT_DETACH)
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: mounting work tmpfs: %w", err)
	}

	// Assemble the OverlayFS. OverlayFS searches lower dirs left-to-right,
	// so the highest-MountOrder layer must appear first in lowerdir.
	// squashPoints is in ascending MountOrder order, so reverse it.
	highestFirst := make([]string, len(squashPoints))
	for i, p := range squashPoints {
		highestFirst[len(squashPoints)-1-i] = p
	}
	lowerDir := strings.Join(highestFirst, ":")
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, strataUpperDir, strataWorkDir)

	if err := os.MkdirAll(strataMergedDir, 0755); err != nil {
		_ = syscall.Unmount(strataWorkDir, syscall.MNT_DETACH)
		_ = syscall.Unmount(strataUpperDir, syscall.MNT_DETACH)
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: creating merged dir: %w", err)
	}
	if err := syscall.Mount("overlay", strataMergedDir, "overlay", 0, opts); err != nil {
		_ = syscall.Unmount(strataWorkDir, syscall.MNT_DETACH)
		_ = syscall.Unmount(strataUpperDir, syscall.MNT_DETACH)
		cleanupSquash(squashPoints)
		return nil, fmt.Errorf("overlay: mounting OverlayFS: %w", err)
	}

	return &Overlay{
		MergedPath:        strataMergedDir,
		UpperDir:          strataUpperDir,
		WorkDir:           strataWorkDir,
		squashMountPoints: squashPoints,
	}, nil
}

// Cleanup unmounts in reverse order: overlay first, then tmpfs dirs, then
// squashfs mounts. MNT_DETACH (lazy unmount) handles busy mounts gracefully.
// The first error encountered is returned, but cleanup continues regardless.
func (o *Overlay) Cleanup() error {
	if o == nil {
		return nil
	}
	var firstErr error
	record := func(err error) {
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	record(syscall.Unmount(o.MergedPath, syscall.MNT_DETACH))
	record(syscall.Unmount(o.UpperDir, syscall.MNT_DETACH))
	record(syscall.Unmount(o.WorkDir, syscall.MNT_DETACH))
	for i := len(o.squashMountPoints) - 1; i >= 0; i-- {
		record(syscall.Unmount(o.squashMountPoints[i], syscall.MNT_DETACH))
	}
	return firstErr
}

// cleanupSquash unmounts a slice of squashfs mount points in reverse order.
// Errors are silently discarded — this is used only during failed Mount calls.
func cleanupSquash(points []string) {
	for i := len(points) - 1; i >= 0; i-- {
		_ = syscall.Unmount(points[i], syscall.MNT_DETACH)
	}
}
