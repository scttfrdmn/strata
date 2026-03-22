//go:build !linux

package fold

import (
	"context"
	"errors"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// ErrNotSupported is returned when squashfs merging is attempted on a
// non-Linux platform. Use --eject for a plain directory alternative.
var ErrNotSupported = errors.New("fold: squashfs merge requires Linux; use --eject for a plain directory")

// MergeToLayer is not supported on non-Linux platforms.
func MergeToLayer(_ context.Context, _ *spec.LockFile, _ []overlay.LayerPath, _ MergeConfig) (*spec.LayerManifest, error) {
	return nil, ErrNotSupported
}
