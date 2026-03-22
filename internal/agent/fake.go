package agent

import (
	"context"
	"fmt"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// FakeLockfileSource returns a pre-configured lockfile or error.
// Use it in tests to avoid EC2 metadata or S3 dependencies.
type FakeLockfileSource struct {
	Lockfile *spec.LockFile
	Err      error
}

// Acquire returns the pre-configured Lockfile and Err.
func (f *FakeLockfileSource) Acquire(_ context.Context) (*spec.LockFile, error) {
	return f.Lockfile, f.Err
}

// FakeLayerFetcher returns pre-configured local paths keyed by layer ID.
// Use it in tests to avoid S3 downloads.
type FakeLayerFetcher struct {
	// Paths maps layer ID to local file path.
	Paths map[string]string
	// Err, if non-nil, is returned for every Fetch call.
	Err error
}

// Fetch returns the path for layer.ID from Paths, or Err if set.
func (f *FakeLayerFetcher) Fetch(_ context.Context, layer spec.ResolvedLayer) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	path, ok := f.Paths[layer.ID]
	if !ok {
		return "", fmt.Errorf("FakeLayerFetcher: no path configured for layer ID %q", layer.ID)
	}
	return path, nil
}

// FakeReadySignaler records calls to SignalReady and SignalFailed for assertion
// in tests. It never returns an error.
type FakeReadySignaler struct {
	ReadyCalled  bool
	FailedCalled bool
	FailedReason error
}

// SignalReady records that readiness was signaled.
func (f *FakeReadySignaler) SignalReady(_ context.Context, _ *spec.LockFile) error {
	f.ReadyCalled = true
	return nil
}

// SignalFailed records the failure reason.
func (f *FakeReadySignaler) SignalFailed(_ context.Context, reason error) error {
	f.FailedCalled = true
	f.FailedReason = reason
	return nil
}

// FakeMounter returns a pre-built Overlay or error.
// Use it in tests to avoid Linux OverlayFS syscalls.
type FakeMounter struct {
	Result *overlay.Overlay
	Err    error
}

// Mount returns the pre-configured Result and Err.
func (f *FakeMounter) Mount(_ []overlay.LayerPath) (*overlay.Overlay, error) {
	return f.Result, f.Err
}

// FakePackageInstaller records Install calls for assertion in tests.
// It never returns an error unless Err is set.
type FakePackageInstaller struct {
	Called     bool
	MergedPath string
	Pkgs       []spec.ResolvedPackageSet
	Err        error
}

// Install records the call and returns Err.
func (f *FakePackageInstaller) Install(_ context.Context, pkgs []spec.ResolvedPackageSet, mergedPath string) error {
	f.Called = true
	f.MergedPath = mergedPath
	f.Pkgs = pkgs
	return f.Err
}
