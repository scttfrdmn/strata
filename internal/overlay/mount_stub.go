//go:build !linux

package overlay

// Mount always returns ErrNotSupported on non-Linux platforms.
// OverlayFS assembly requires Linux kernel support.
func Mount(_ []LayerPath) (*Overlay, error) { return nil, ErrNotSupported }

// MountWithConfig always returns ErrNotSupported on non-Linux platforms.
func MountWithConfig(_ []LayerPath, _ Config) (*Overlay, error) { return nil, ErrNotSupported }

// MountBuildEnv always returns ErrNotSupported on non-Linux platforms.
func MountBuildEnv(_ []LayerPath, _ string) (*Overlay, error) { return nil, ErrNotSupported }

// Cleanup always returns ErrNotSupported on non-Linux platforms.
func (o *Overlay) Cleanup() error { return ErrNotSupported }
