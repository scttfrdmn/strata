//go:build linux

package overlay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	strataLayersDir = "/strata/layers"
	strataRWDir     = "/strata/rw"       // single tmpfs; upper and work live inside
	strataUpperDir  = "/strata/rw/upper" // OverlayFS upper (ephemeral writes)
	strataWorkDir   = "/strata/rw/work"  // OverlayFS internal
	strataMergedDir = "/strata/env"
)

// ————— syscallMountStrategy —————

// syscallMountStrategy implements MountStrategy using Linux kernel syscalls.
// Requires CAP_SYS_ADMIN (root or appropriate capabilities).
type syscallMountStrategy struct{}

func (syscallMountStrategy) MountSquashfs(sqfsPath, mp string) error {
	return mountSquashfs(sqfsPath, mp)
}

func (syscallMountStrategy) MountTmpfs(mp string) error {
	return syscall.Mount("tmpfs", mp, "tmpfs", 0, "size=1g,mode=755")
}

func (syscallMountStrategy) MountOverlay(lowerDirs []string, upper, work, merged string) error {
	highestFirst := make([]string, len(lowerDirs))
	copy(highestFirst, lowerDirs)
	reverseStrings(highestFirst)
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(highestFirst, ":"), upper, work)
	return syscall.Mount("overlay", merged, "overlay", 0, opts)
}

func (syscallMountStrategy) Unmount(mp string, _ bool) error {
	return syscall.Unmount(mp, syscall.MNT_DETACH)
}

// ————— strategy selection —————

// selectMountStrategy auto-detects the appropriate mount strategy.
// In container environments or when CAP_SYS_ADMIN is unavailable, it
// falls back to FUSE tools (squashfuse + fuse-overlayfs).
func selectMountStrategy() (MountStrategy, error) {
	// Container hints: avoid a blocking syscall attempt if we know we're inside
	// a container that won't have CAP_SYS_ADMIN.
	inContainer := os.Getenv("container") != "" || fileExists("/run/.containerenv")
	if !inContainer && canSyscallMount() {
		return syscallMountStrategy{}, nil
	}
	if hasFUSETools() {
		return fuseMountStrategy{}, nil
	}
	return nil, fmt.Errorf("overlay: no mount strategy available — need CAP_SYS_ADMIN or squashfuse+fuse-overlayfs in PATH")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// canSyscallMount attempts a harmless test mount to check for CAP_SYS_ADMIN.
func canSyscallMount() bool {
	// Try to mount a tmpfs on /proc/self/fd — this is a reliable privilege check
	// that fails immediately with EPERM when CAP_SYS_ADMIN is absent.
	// We use a clearly invalid target so the mount cannot succeed even if
	// permissions were granted, which avoids any side effects.
	err := syscall.Mount("none", "/dev/null", "tmpfs", 0, "")
	// ENOTDIR means we had permission but /dev/null isn't a directory — OK.
	// EPERM/EACCES mean no capability.
	return err == nil || err == syscall.ENOTDIR
}

// hasFUSETools returns true when both squashfuse and fuse-overlayfs are in PATH.
func hasFUSETools() bool {
	_, squashErr := exec.LookPath("squashfuse")
	_, fuseErr := exec.LookPath("fuse-overlayfs")
	return squashErr == nil && fuseErr == nil
}

// ————— MountWithConfig —————

// MountWithConfig assembles the OverlayFS using the provided Config.
// Zero-value Config fields use production defaults (/strata/* paths,
// auto-detected mount strategy).
//
// Layers are mounted in ascending MountOrder (MountOrder=1 is at the bottom
// of the OverlayFS lower stack). The highest-MountOrder layer wins in the
// OverlayFS lookup order.
//
// If any mount step fails, all previously mounted filesystems are unmounted
// before returning the error. No dangling mounts are left behind.
func MountWithConfig(layers []LayerPath, cfg Config) (*Overlay, error) {
	// Apply defaults.
	layersDir := cfg.LayersDir
	if layersDir == "" {
		layersDir = strataLayersDir
	}
	rwDir := cfg.RWDir
	if rwDir == "" {
		rwDir = strataRWDir
	}
	mergedDir := cfg.MergedDir
	if mergedDir == "" {
		mergedDir = strataMergedDir
	}
	upperDir := filepath.Join(rwDir, "upper")
	workDir := filepath.Join(rwDir, "work")

	strategy := cfg.Strategy
	if strategy == nil {
		var err error
		strategy, err = selectMountStrategy()
		if err != nil {
			return nil, err
		}
	}

	// Sort ascending by MountOrder so we mount bottom-of-stack first.
	sorted := make([]LayerPath, len(layers))
	copy(sorted, layers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MountOrder < sorted[j].MountOrder
	})

	// Mount each layer as squashfs.
	var squashPoints []string
	for _, layer := range sorted {
		mp := filepath.Join(layersDir, layer.ID)
		if err := os.MkdirAll(mp, 0755); err != nil {
			cleanupWith(strategy, squashPoints)
			return nil, fmt.Errorf("overlay: creating mount point for %q: %w", layer.ID, err)
		}
		if err := strategy.MountSquashfs(layer.Path, mp); err != nil {
			cleanupWith(strategy, squashPoints)
			return nil, fmt.Errorf("overlay: mounting squashfs %q at %q: %w", layer.ID, mp, err)
		}
		squashPoints = append(squashPoints, mp)
	}

	// rw dir: tmpfs (syscall) or regular directory (FUSE).
	if err := os.MkdirAll(rwDir, 0755); err != nil {
		cleanupWith(strategy, squashPoints)
		return nil, fmt.Errorf("overlay: creating rw dir: %w", err)
	}
	if err := strategy.MountTmpfs(rwDir); err != nil {
		cleanupWith(strategy, squashPoints)
		return nil, fmt.Errorf("overlay: mounting rw tmpfs: %w", err)
	}

	if err := os.MkdirAll(upperDir, 0755); err != nil {
		cleanupRW(strategy, rwDir, squashPoints)
		return nil, fmt.Errorf("overlay: creating upper dir: %w", err)
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		cleanupRW(strategy, rwDir, squashPoints)
		return nil, fmt.Errorf("overlay: creating work dir: %w", err)
	}

	if err := os.MkdirAll(mergedDir, 0755); err != nil {
		cleanupRW(strategy, rwDir, squashPoints)
		return nil, fmt.Errorf("overlay: creating merged dir: %w", err)
	}
	if err := strategy.MountOverlay(squashPoints, upperDir, workDir, mergedDir); err != nil {
		cleanupRW(strategy, rwDir, squashPoints)
		return nil, fmt.Errorf("overlay: mounting OverlayFS: %w", err)
	}

	isFUSE := false
	if _, ok := strategy.(fuseMountStrategy); ok {
		isFUSE = true
	}
	_ = isFUSE // used via o.strategy field check in Cleanup

	return &Overlay{
		MergedPath:        mergedDir,
		UpperDir:          upperDir,
		WorkDir:           workDir,
		rwMount:           rwDir,
		squashMountPoints: squashPoints,
		strategy:          strategy,
	}, nil
}

// Mount mounts each layer as a read-only squashfs loopback device, creates a
// tmpfs for the upper and work directories, then assembles the OverlayFS at
// /strata/env. It is a thin wrapper around MountWithConfig with production defaults.
func Mount(layers []LayerPath) (*Overlay, error) {
	return MountWithConfig(layers, Config{})
}

// MountBuildEnv mounts the given squashfs layers as a read-only OverlayFS
// build environment at a configurable base directory. It is a thin wrapper
// around MountWithConfig with paths under baseDir.
//
// Layout under baseDir:
//
//	layers/<id>  — squashfs mount point for each layer (read-only)
//	rw/upper     — tmpfs subdir for ephemeral writes during build
//	rw/work      — OverlayFS internal work directory (tmpfs)
//	merged       — merged view presented to the build script
func MountBuildEnv(layers []LayerPath, baseDir string) (*Overlay, error) {
	return MountWithConfig(layers, Config{
		LayersDir: filepath.Join(baseDir, "layers"),
		RWDir:     filepath.Join(baseDir, "rw"),
		MergedDir: filepath.Join(baseDir, "merged"),
		Strategy:  syscallMountStrategy{}, // build env always uses syscalls
	})
}

// Cleanup unmounts in reverse order: overlay first, then tmpfs dirs, then
// squashfs mounts. The first error encountered is returned, but cleanup
// continues regardless.
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

	if _, isFUSE := o.strategy.(fuseMountStrategy); isFUSE {
		// FUSE cleanup: fusermount for overlay + squashfs; rwMount is just a dir.
		record(o.strategy.Unmount(o.MergedPath, true))
		// rwMount was created with os.MkdirAll — no unmount needed; the caller
		// (strata run) cleans up the temp dir, or agent leaves it in place.
		for i := len(o.squashMountPoints) - 1; i >= 0; i-- {
			record(o.strategy.Unmount(o.squashMountPoints[i], true))
		}
		return firstErr
	}

	// Syscall cleanup (default for nil strategy and explicit syscallMountStrategy).
	record(syscall.Unmount(o.MergedPath, syscall.MNT_DETACH))
	if o.rwMount != "" {
		record(syscall.Unmount(o.rwMount, syscall.MNT_DETACH))
	} else {
		record(syscall.Unmount(o.UpperDir, syscall.MNT_DETACH))
		record(syscall.Unmount(o.WorkDir, syscall.MNT_DETACH))
	}
	for i := len(o.squashMountPoints) - 1; i >= 0; i-- {
		record(syscall.Unmount(o.squashMountPoints[i], syscall.MNT_DETACH))
	}
	return firstErr
}

// ————— helpers —————

func reverseStrings(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// cleanupWith unmounts a list of squashfs mount points using the given strategy.
func cleanupWith(strategy MountStrategy, points []string) {
	for i := len(points) - 1; i >= 0; i-- {
		_ = strategy.Unmount(points[i], true)
	}
}

// cleanupRW unmounts the rw dir (syscall or no-op for FUSE) plus squash points.
func cleanupRW(strategy MountStrategy, rwDir string, squashPoints []string) {
	if _, isFUSE := strategy.(fuseMountStrategy); !isFUSE {
		_ = syscall.Unmount(rwDir, syscall.MNT_DETACH)
	}
	cleanupWith(strategy, squashPoints)
}

// mountSquashfs mounts a squashfs file at mp read-only via the userspace
// mount(8) command. Using the syscall directly with "loop" data doesn't work
// because the kernel mount(2) doesn't set up loop devices — only mount(8) does.
func mountSquashfs(sqfsPath, mp string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("mount", "-t", "squashfs", "-o", "loop,ro", sqfsPath, mp)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}
