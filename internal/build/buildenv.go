package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scttfrdmn/strata/spec"
)

// EnvLayer is a resolved build_requires layer with its local .sqfs path,
// ready for OverlayFS mounting as a build environment.
type EnvLayer struct {
	// Manifest is the full layer manifest from the registry.
	Manifest *spec.LayerManifest

	// SqfsPath is the local path to the downloaded squashfs file.
	SqfsPath string

	// MountOrder is the layer's position in the OverlayFS lower stack (1 = bottom).
	MountOrder int
}

// RegistryClient is the narrow registry interface needed by
// RegistryBuildEnvResolver. Satisfied by *registry.S3Client.
type RegistryClient interface {
	// ResolveLayer returns the highest-versioned layer manifest matching name,
	// versionPrefix, arch, and family.
	ResolveLayer(ctx context.Context, name, versionPrefix, arch, family string) (*spec.LayerManifest, error)

	// FetchLayerSqfs downloads the squashfs file for manifest to cacheDir and
	// returns the local file path. Cache hit returns the existing file immediately.
	FetchLayerSqfs(ctx context.Context, manifest *spec.LayerManifest, cacheDir string) (string, error)
}

// EnvResolver resolves build_requires requirements to local squashfs files,
// ready for OverlayFS mounting as a build environment. This is Stage 3 of the
// build pipeline.
type EnvResolver interface {
	// Resolve fetches manifests matching buildRequires (arch + family) from the
	// registry, downloads their squashfs files to cacheDir, and returns the
	// resolved layers in mount order (lowest-tier layer first).
	Resolve(ctx context.Context, buildRequires []spec.Requirement, arch, family, cacheDir string) ([]EnvLayer, error)
}

// RegistryBuildEnvResolver resolves and downloads build environment layers from
// a Strata registry. Satisfies EnvResolver.
type RegistryBuildEnvResolver struct {
	Registry RegistryClient
}

// Resolve fetches and downloads each build_requires layer in order.
// The returned layers are indexed by their position in buildRequires; the
// first requirement gets MountOrder=1 (lowest in the OverlayFS stack).
func (r *RegistryBuildEnvResolver) Resolve(ctx context.Context, buildRequires []spec.Requirement, arch, family, cacheDir string) ([]EnvLayer, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("buildenv: creating cache dir %q: %w", cacheDir, err)
	}

	result := make([]EnvLayer, 0, len(buildRequires))
	for i, req := range buildRequires {
		m, err := r.Registry.ResolveLayer(ctx, req.Name, req.MinVersion, arch, family)
		if err != nil {
			return nil, fmt.Errorf("buildenv: resolving build_requires %q: %w", req.String(), err)
		}

		sqfsPath, err := r.Registry.FetchLayerSqfs(ctx, m, cacheDir)
		if err != nil {
			return nil, fmt.Errorf("buildenv: downloading layer %q: %w", m.ID, err)
		}

		result = append(result, EnvLayer{
			Manifest:   m,
			SqfsPath:   sqfsPath,
			MountOrder: i + 1,
		})
	}
	return result, nil
}

// defaultLayerCacheDir returns the default cache directory for downloaded
// build environment layers.
func defaultLayerCacheDir() string {
	return filepath.Join(os.TempDir(), "strata-build-cache")
}

// FakeBuildEnvResolver returns pre-configured EnvLayers for testing.
// It never calls the registry or downloads any files.
type FakeBuildEnvResolver struct {
	// Layers maps requirement name (e.g. "gcc") to the EnvLayer to return.
	// The same layer is returned regardless of version constraint.
	Layers map[string]EnvLayer
}

// Resolve returns pre-configured layers for each requirement.
// Returns an error if a requirement has no matching entry in Layers.
func (f *FakeBuildEnvResolver) Resolve(_ context.Context, buildRequires []spec.Requirement, _, _, _ string) ([]EnvLayer, error) {
	result := make([]EnvLayer, 0, len(buildRequires))
	for i, req := range buildRequires {
		layer, ok := f.Layers[req.Name]
		if !ok {
			return nil, fmt.Errorf("FakeBuildEnvResolver: no layer configured for %q", req.String())
		}
		layer.MountOrder = i + 1
		result = append(result, layer)
	}
	return result, nil
}
