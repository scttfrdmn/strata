//go:build !linux

package agent

import (
	"context"
	"errors"

	"github.com/scttfrdmn/strata/spec"
)

// ErrPackageInstallNotSupported is returned on non-Linux platforms where
// package installation into an overlay is not available.
var ErrPackageInstallNotSupported = errors.New("agent: package installation requires Linux")

// ExecPackageInstaller is the production PackageInstaller implementation.
// On non-Linux platforms it always returns ErrPackageInstallNotSupported.
type ExecPackageInstaller struct{}

// Install always returns ErrPackageInstallNotSupported on non-Linux platforms.
func (ExecPackageInstaller) Install(_ context.Context, _ []spec.ResolvedPackageSet, _ string) error {
	return ErrPackageInstallNotSupported
}
