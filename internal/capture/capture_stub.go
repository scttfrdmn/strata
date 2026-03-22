//go:build !linux

// Package capture snapshots an installed prefix into a signed squashfs layer.
// This stub returns ErrNotSupported on non-Linux platforms.
package capture

import (
	"context"
	"errors"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// ErrNotSupported is returned on non-Linux platforms.
var ErrNotSupported = errors.New("capture: strata capture requires Linux")

// PushRegistry is the narrow interface capture needs from the registry.
// Satisfied by *registry.S3Client and *registry.LocalClient.
type PushRegistry interface {
	PushLayer(ctx context.Context, manifest *spec.LayerManifest,
		sqfsPath string, bundleJSON []byte) error
}

// Config holds the configuration for a capture operation.
type Config struct {
	Name, Version, Prefix string
	ABI, Arch             string
	Normalize             bool
	CaptureSource         string
	OriginalPrefix        string
	Signer                trust.Signer
	Registry              PushRegistry
	DryRun                bool
	TempDir               string
	Provides              []spec.Capability
	Requires              []spec.Requirement
}

// Result is returned on a successful capture.
type Result struct {
	Manifest    *spec.LayerManifest
	SqfsPath    string
	RegistryURI string
}

// Capture returns ErrNotSupported on non-Linux platforms.
func Capture(_ context.Context, _ Config) (*Result, error) {
	return nil, ErrNotSupported
}
