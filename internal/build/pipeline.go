package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// PushRegistry is the narrow interface the pipeline needs from the registry.
// Satisfied by *registry.S3Client. Defined here (consumer side) to enable
// lightweight mock injection in pipeline tests.
type PushRegistry interface {
	PushLayer(ctx context.Context, manifest *spec.LayerManifest,
		sqfsPath string, bundleJSON []byte) error
}

// Run executes the local build pipeline for a single recipe.
//
// Stages 2 (EC2 launch), 3 (overlay mount), and 11 (terminate) are skipped
// in v0.9.0 — builds run on the local machine. Pass job.DryRun=true to
// validate and print a summary without executing.
//
// reg may be nil when job.DryRun is true.
func Run(
	ctx context.Context,
	job *Job,
	recipe *Recipe,
	reg PushRegistry,
	executor Executor,
	signer trust.Signer,
) (*spec.LayerManifest, error) {
	if err := job.Validate(); err != nil {
		return nil, err
	}

	arch := job.Base.NormalizedArch()
	manifest := recipe.Meta.ToLayerManifest(arch)

	// DryRun path — validate and return a sentinel manifest.
	if job.DryRun {
		manifest.SHA256 = "dry-run"
		manifest.RekorEntry = "dry-run"
		fmt.Fprintf(os.Stderr, "dry-run: recipe:  %s@%s (%s/%s)\n",
			recipe.Meta.Name, recipe.Meta.Version, recipe.Meta.Family, arch)
		fmt.Fprintf(os.Stderr, "dry-run: script:  %s\n", recipe.BuildScriptPath)
		return manifest, nil
	}

	if reg == nil {
		return nil, fmt.Errorf("build: registry required for non-dry-run builds")
	}

	// Stage 3 — resolve and mount build_requires via OverlayFS if an EnvResolver
	// is configured. Falls back to bootstrap mode (Tier 0) when absent.
	baseEnv := []string{
		"STRATA_NCPUS=" + strconv.Itoa(runtime.NumCPU()),
		"STRATA_ARCH=" + arch,
	}
	buildEnvCleanup, envVars, err := prepareStage3(ctx, job, recipe, arch, manifest)
	if err != nil {
		return nil, err
	}
	baseEnv = append(baseEnv, envVars...)

	// Stage 4 — create output dir, set env, execute build script.
	outputDir, err := os.MkdirTemp("", "strata-build-*")
	if err != nil {
		if buildEnvCleanup != nil {
			buildEnvCleanup()
		}
		return nil, fmt.Errorf("build: creating output dir: %w", err)
	}
	// outputDir is removed after squashfs is created (stage 5).

	installPrefix := filepath.Join(outputDir, recipe.Meta.Name, recipe.Meta.Version)
	if err := os.MkdirAll(installPrefix, 0o755); err != nil {
		os.RemoveAll(outputDir) //nolint:errcheck
		if buildEnvCleanup != nil {
			buildEnvCleanup()
		}
		return nil, fmt.Errorf("build: creating install prefix: %w", err)
	}

	env := append(baseEnv,
		"STRATA_PREFIX="+outputDir,
		"STRATA_INSTALL_PREFIX="+installPrefix,
		"STRATA_OUT="+outputDir,
	)
	if err := executor.Execute(ctx, recipe.BuildScriptPath, env, outputDir); err != nil {
		os.RemoveAll(outputDir) //nolint:errcheck
		if buildEnvCleanup != nil {
			buildEnvCleanup()
		}
		return nil, fmt.Errorf("build: executing recipe: %w", err)
	}

	// Stage 8 — generate content manifest BEFORE removing outputDir.
	contentManifest, err := GenerateContentManifest(outputDir, manifest.ID)
	if err != nil {
		os.RemoveAll(outputDir) //nolint:errcheck
		return nil, fmt.Errorf("build: generating content manifest: %w", err)
	}
	manifest.ContentManifest = contentManifest.Files

	// Stage 5 — create squashfs, then free the output dir.
	sqfsFile, err := os.CreateTemp("", "strata-layer-*.sqfs")
	if err != nil {
		os.RemoveAll(outputDir) //nolint:errcheck
		return nil, fmt.Errorf("build: creating sqfs temp file: %w", err)
	}
	sqfsPath := sqfsFile.Name()
	sqfsFile.Close()          //nolint:errcheck
	defer os.Remove(sqfsPath) //nolint:errcheck

	if err := CreateSquashfs(ctx, outputDir, sqfsPath); err != nil {
		os.RemoveAll(outputDir) //nolint:errcheck
		if buildEnvCleanup != nil {
			buildEnvCleanup()
		}
		return nil, fmt.Errorf("build: creating squashfs: %w", err)
	}
	os.RemoveAll(outputDir) //nolint:errcheck
	if buildEnvCleanup != nil {
		buildEnvCleanup() // unmount build env after sqfs is created
	}

	// Stage 6 — SHA256 of squashfs.
	sha256hex, err := sha256HexFile(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("build: hashing sqfs: %w", err)
	}
	manifest.SHA256 = sha256hex

	// Stage 7 — size of squashfs.
	info, err := os.Stat(sqfsPath)
	if err != nil {
		return nil, fmt.Errorf("build: stating sqfs: %w", err)
	}
	manifest.Size = info.Size()

	// Stage 9 — sign with cosign → Rekor.
	manifest.RecipeSHA256, err = sha256HexFile(recipe.BuildScriptPath)
	if err != nil {
		return nil, fmt.Errorf("build: hashing build script: %w", err)
	}
	manifest.BuiltAt = time.Now().UTC()
	manifest.CosignVersion = trust.CosignToolVersion(ctx)

	annotations := map[string]string{
		"strata.layer.name":          recipe.Meta.Name,
		"strata.layer.version":       recipe.Meta.Version,
		"strata.layer.family":        recipe.Meta.Family,
		"strata.layer.arch":          arch,
		"strata.layer.recipe_sha256": manifest.RecipeSHA256,
	}
	bundle, err := signer.Sign(ctx, sqfsPath, annotations)
	if err != nil {
		return nil, fmt.Errorf("build: signing layer: %w", err)
	}

	bundleJSON, err := bundle.Marshal()
	if err != nil {
		return nil, fmt.Errorf("build: marshaling bundle: %w", err)
	}

	if logIdx, ok := bundle.RekorLogIndex(); ok {
		manifest.RekorEntry = strconv.FormatInt(logIdx, 10)
	}

	// Stage 10 — push to registry.
	if err := reg.PushLayer(ctx, manifest, sqfsPath, bundleJSON); err != nil {
		return nil, fmt.Errorf("build: pushing layer: %w", err)
	}

	return manifest, nil
}

// prepareStage3 resolves and mounts build_requires layers as an OverlayFS
// build environment. It populates manifest.BuiltWith, manifest.BootstrapBuild,
// and manifest.BuildEnvLockID, then returns:
//
//   - cleanup: unmounts the overlay; nil when no overlay was mounted
//   - envVars: PATH/LD_LIBRARY_PATH env vars pointing at the merged overlay dir
//   - err: non-nil if resolution or mounting failed
//
// When job.EnvResolver is nil or recipe.Meta.BuildRequires is empty, the layer
// is classified as a bootstrap build (Tier 0): manifest.BootstrapBuild = true
// and cleanup/envVars are both nil.
func prepareStage3(ctx context.Context, job *Job, recipe *Recipe, arch string, manifest *spec.LayerManifest) (cleanup func(), envVars []string, err error) {
	if job.EnvResolver == nil || len(recipe.Meta.BuildRequires) == 0 {
		manifest.BootstrapBuild = true
		return nil, nil, nil
	}

	cacheDir := job.CacheDir
	if cacheDir == "" {
		cacheDir = defaultLayerCacheDir()
	}

	layers, err := job.EnvResolver.Resolve(ctx, recipe.Meta.BuildRequires, arch, recipe.Meta.Family, cacheDir)
	if err != nil {
		return nil, nil, fmt.Errorf("build: resolving build environment: %w", err)
	}

	// Convert EnvLayer slice to overlay.LayerPath slice.
	layerPaths := make([]overlay.LayerPath, len(layers))
	builtWith := make([]spec.LayerRef, len(layers))
	for i, l := range layers {
		layerPaths[i] = overlay.LayerPath{
			ID:         l.Manifest.ID,
			SHA256:     l.Manifest.SHA256,
			Path:       l.SqfsPath,
			MountOrder: l.MountOrder,
		}
		builtWith[i] = spec.LayerRef{
			Name:    l.Manifest.ID,
			Version: l.Manifest.Version,
			SHA256:  l.Manifest.SHA256,
			Rekor:   l.Manifest.RekorEntry,
		}
	}

	// Create a per-job temp dir for the build overlay so multiple concurrent
	// builds don't conflict with each other or with /strata/env.
	baseDir, err := os.MkdirTemp("", "strata-buildenv-*")
	if err != nil {
		return nil, nil, fmt.Errorf("build: creating build env base dir: %w", err)
	}

	ov, mountErr := overlay.MountBuildEnv(layerPaths, baseDir)
	if mountErr != nil {
		os.RemoveAll(baseDir) //nolint:errcheck
		return nil, nil, fmt.Errorf("build: mounting build environment: %w", mountErr)
	}

	manifest.BuiltWith = builtWith
	manifest.BuildEnvLockID = buildEnvLockID(builtWith)

	cleanupFn := func() {
		_ = ov.Cleanup()
		os.RemoveAll(baseDir) //nolint:errcheck
	}

	// Build PATH and LD_LIBRARY_PATH from the actual install layout of each
	// layer. The pipeline installs each layer to <name>/<version>/ inside the
	// squashfs root, so binaries live at <mergedPath>/<name>/<version>/bin/.
	var pathDirs, ldDirs []string
	for _, l := range layers {
		base := ov.MergedPath + "/" + l.Manifest.Name + "/" + l.Manifest.Version
		pathDirs = append(pathDirs, base+"/bin")
		ldDirs = append(ldDirs, base+"/lib", base+"/lib64")
	}
	// Append system dirs as fallback.
	pathDirs = append(pathDirs, "/usr/local/bin", "/usr/bin", "/bin")
	ldDirs = append(ldDirs, "/usr/lib", "/usr/lib64")

	vars := []string{
		"PATH=" + strings.Join(pathDirs, ":"),
		"LD_LIBRARY_PATH=" + strings.Join(ldDirs, ":"),
		"STRATA_BUILD_ENV=" + ov.MergedPath,
	}

	return cleanupFn, vars, nil
}

// buildEnvLockID returns the SHA256 of the YAML-serialized BuiltWith list.
// This is stored as manifest.BuildEnvLockID so reviewers can independently
// verify the exact build environment from the manifest.
func buildEnvLockID(builtWith []spec.LayerRef) string {
	data, _ := yaml.Marshal(builtWith)
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
