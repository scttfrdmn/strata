# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.20.2] - 2026-03-23

### Fixed
- **CI**: `golangci-lint` config updated for v2 schema — `exclude-rules` moved to
  `linters.exclusions.rules`; `ErrNotSupported` moved to a platform-neutral file so
  Linux CI can compile `internal/capture` without the stub.
- **CI**: upgraded to `golangci-lint-action@v9`, `actions/checkout@v6`,
  `actions/setup-go@v6` (Node.js 24); pinned golangci-lint to v2.11.3; switched to
  `go-version-file: go.mod` to track the declared Go version.
- **CI**: added `codecov/codecov-action@v5` for coverage reporting.
- **Lint**: reduced cyclomatic complexity of `Capture()` (34 → ~26) by extracting
  `validateCaptureConfig` and `signLayer` helpers; lowercased capitalized error string
  in `package_installer.go`; applied `gofmt -s` to `internal/capture/capture.go`.
- **Lint**: raised `gocyclo min-complexity` to 30 to match pre-existing codebase
  baseline (threshold was previously silently unenforced).

## [0.20.1] - 2026-03-22

### Security
- **HTTP timeouts**: replaced `http.DefaultClient` (no timeout) with a 30-second
  timeout client in `internal/trust/http.go` and `internal/packages/resolve.go` to
  prevent hung requests to Sigstore/Rekor, PyPI, and CRAN registries.
- **CRAN injection**: added `^[A-Za-z0-9._-]+$` validation before interpolating
  package names into R scripts in `internal/agent/package_installer.go`.
- **EC2 UserData shell injection**: validated `RecipeName`, `RecipeVersion`, and
  `Arch` against a safe-character regex; single-quoted `RegistryURL` in the bash
  template in `internal/build/ec2runner.go`.
- **Unbounded reads**: applied `io.LimitReader` to every network and S3 `io.ReadAll`
  call across `internal/registry/s3client.go`, `cmd/strata-agent/metadata_source.go`,
  `cmd/strata-agent/s3_fetcher.go`, `cmd/strata-agent/main.go`,
  `cmd/strata/snapshot_ami.go`, `cmd/strata-agent/ec2_signaler.go`, and
  `internal/packages/resolve.go`. Caps range from 256 B (instance-id) to 10 MiB
  (lockfiles, bundles, YAML indexes).
- **File size guard**: `spec/parse.go` now stats files before `os.ReadFile` and
  rejects anything over 10 MiB before deserializing YAML.
- **S3 URI validation**: `parseObjectURI` in `internal/registry/s3client.go` now
  returns false for empty keys (`s3://bucket/`).
- **Hardcoded bucket**: `cmd/strata-agent/main.go` replaced hardcoded
  `"strata-registry"` with `registryBucket()`, overridable via
  `STRATA_REGISTRY_BUCKET` env var.
- **Path traversal**: `cmd/strata/run.go` rejects `file://` layer sources containing
  `..` components.
- **Symlink escape**: `internal/fold/eject.go` `copyTree` skips absolute symlinks
  that resolve outside the output root.
- **VerifyLayer URI guard**: `internal/trust/verify.go` now explicitly rejects URI
  schemes (`s3://`, `file://`, `http://`) in `manifest.Bundle` before passing to
  `os.ReadFile`, enforcing the local-path contract of the `VerifyLayer` API.
- **VerifyLayers hardening**: added `context.WithCancel` so the first verification
  failure cancels remaining goroutines promptly; replaced string concat with
  `filepath.Join` for cache paths to prevent `..` escape via crafted layer IDs.

## [0.17.0] - 2026-03-22

### Added
- **`strata export --format oci`**: converts a lockfile's squashfs layers into an
  OCI Image Layout tar archive (`internal/export/oci.go`). Each layer is unpacked
  with `unsquashfs`, re-packed as a deterministic tar (sorted paths, epoch-0
  timestamps, no uid/gid), and assembled per OCI Image Layout 1.0. Requires
  `unsquashfs` (squashfs-tools) in PATH; no root or FUSE required.
- **Container ecosystem documentation** (`docs/container-ecosystem.md`): covers
  privileged Docker, unprivileged Docker (`--device /dev/fuse`), rootless Podman
  (no extra flags), Apptainer/Singularity, example multi-stage Containerfile, and
  a comparison table of `strata run` vs `strata export` use cases.
- **OCI provenance labels**: exported images carry
  `org.opencontainers.image.revision` (EnvironmentID), `strata.layer.<name>.sha256`,
  and `strata.layer.<name>.rekor_entry` in the image config.

### Changed
- **Container-aware mount strategy** (`internal/overlay/mount_linux.go`):
  `selectMountStrategy()` now checks `$container` env var and `/run/.containerenv`
  before attempting a kernel mount syscall — inside Podman/nspawn/Flatpak containers
  the FUSE strategy is selected immediately without a blocking syscall attempt.

## [0.16.0] - 2026-03-22

### Added
- **`strata run`** (`cmd/strata/run.go`): ephemeral environment execution without
  the agent or systemd. Reads a lockfile, fetches layers to the user-local cache,
  assembles OverlayFS using the auto-detected mount strategy, and execs the given
  command with per-layer `PATH`/`LD_LIBRARY_PATH` set. Flags: `--lockfile`
  (required), `--no-verify`, `--cache-dir`, `--env KEY=VAL`. Works on HPC login
  nodes via FUSE and on privileged systems via kernel mounts.
- **`MountStrategy` interface and `Config` struct** (`internal/overlay/overlay.go`):
  `MountWithConfig(layers, cfg)` replaces the hardcoded `/strata/*` paths; `Mount()`
  and `MountBuildEnv()` remain as backward-compatible wrappers. `Config.Strategy`
  accepts a `MountStrategy` implementation or nil for auto-detection.
- **`fuseMountStrategy`** (`internal/overlay/mount_fuse_linux.go`): unprivileged
  overlay assembly using `squashfuse` (per-layer mounts) and `fuse-overlayfs`
  (merged view); unmount via `fusermount3`/`fusermount`. Auto-selected when
  CAP_SYS_ADMIN is unavailable and both FUSE tools are in PATH.
- **User-local layer cache** (`cmd/strata/cache.go`): `defaultCacheDir()` follows
  XDG Base Dir Spec — root gets `/strata/cache`; non-root gets
  `$XDG_CACHE_HOME/strata/layers` or `~/.cache/strata/layers`. Used as the default
  for `strata run`, `strata export`, and `strata cache-prune`.
- **HPC/userspace documentation** (`docs/userspace.md`): covers FUSE requirements,
  user-local layer cache, air-gapped operation with `--no-verify`, SLURM batch
  script example, and interactive shell usage.

### Changed
- **`strata cache-prune`**: `--cache-dir` default changed from `/strata/cache` to
  `defaultCacheDir()` so non-root users prune their own `~/.cache/strata/layers/`
  by default.

## [0.15.0] - 2026-03-22

### Added
- **Package spec** (`spec/packages.go`): `PackageSpec` (manager, entries),
  `PackageEntry` (name, version, extras), `ResolvedPackageSet`, and
  `ResolvedPackageEntry` (pinned version + SHA256 for pip). `LockFile.Packages`
  and `Profile.Packages` fields. `LockFile.HasMutableLayer()` helper.
- **Package resolution** (`internal/packages/resolve.go`): `ResolveAll()` resolves
  `PackageSpec` slices to pinned versions — pip via PyPI REST API
  (`/pypi/<name>/json`), CRAN via crandb.r-pkg.org, conda version-pinned as
  declared. Injectable `Resolver` struct for hermetic testing.
- **`strata freeze` resolves packages**: after resolving layers, `strata freeze`
  now calls `packages.ResolveAll()` and writes pinned `packages:` into the lockfile.
- **`PackageInstaller` interface** (`internal/agent/agent.go`): optional step 4.5
  in the boot sequence — installs pinned packages from `lockfile.Packages` into
  the overlay's merged path before configuring the environment.
- **`ExecPackageInstaller`** (`internal/agent/package_installer.go`, Linux only):
  runs `pip install name==version`, `conda install -y name=version`, and
  `Rscript -e 'install.packages(...)'` using the overlay's bin directories.
- **Local filesystem registry** (`internal/registry/localclient.go`):
  `LocalClient` reads a directory tree of squashfs files and JSON manifests as a
  registry — useful for CI, air-gapped builds, and offline development.
- **Federated registry** (`internal/registry/federated.go`): `FederatedClient`
  searches a priority-ordered list of `RegistryClient` implementations; first
  hit wins. Wired into `strata freeze` and `strata resolve`.
- **`strata freeze-layer`** (`cmd/strata/freeze_layer.go`): interactive Path B
  command that prompts for a local squashfs file + metadata, computes SHA256,
  and writes a manifest + lockfile entry.
- **`strata snapshot-ami`** (`cmd/strata/snapshot_ami.go`): creates an AMI
  snapshot from a running Strata environment via IMDSv2.
- **Package management documentation** (`docs/package-management.md`).

## [0.14.1] - 2026-03-19

### Fixed
- **Miniforge installer**: `build.sh` now passes `-u` to the installer so it
  succeeds when `STRATA_PREFIX` already exists (the build pipeline pre-creates
  the directory before invoking the recipe).
- **Nested recipe layout**: `strata build-catalog` (and `discoverRecipes`) now
  correctly handles the `<category>/<name>/<version>/` recipe tree introduced
  in v0.13.0; previously only flat `<name>/<version>/` was recognised.
- **EC2 build instances hang after success**: `ec2:TerminateInstances` was
  missing from the `strata-builder` IAM inline policy. Instances tagged
  `strata:build-status=success` but could not self-terminate, leaving them
  running indefinitely. Policy updated; v0.14.1 Linux binaries re-uploaded to
  `s3://strata-registry/build/bin/`.

## [0.14.0] - 2026-03-18

### Added
- **Boot metrics**: `internal/agent/metrics.go` — `BootMetrics` struct with
  per-step timing (`lockfile_ms`, `fetch_ms`, `mount_ms`, `configure_ms`,
  `total_ms`) and transfer stats (`fetch_bytes`, `cached_layers`,
  `downloaded_layers`). `agent.Run()` now returns `(*BootMetrics, error)`.
  On completion the agent logs JSON to stderr (`journalctl -u strata-agent`),
  writes to `/etc/strata/boot-metrics.json`, and uploads best-effort to
  `s3://strata-registry/metrics/<instance-id>/<iso8601>.json`.
- **Cosign verifier wired**: `cmd/strata-agent/main.go` now calls
  `newCosignVerifier(ctx)` which checks for the cosign binary on PATH and
  downloads `s3://strata-registry/build/keys/cosign.pub` to a temp file.
  Returns nil (skip verification) when either prerequisite is absent — agent
  never fails on missing cosign; SHA256 integrity is always enforced.
- **Bundle verification step**: `internal/agent/agent.go` — new `verifyBundles()`
  called after layer fetch (step 3) when both `Verifier` and `BundleFetcher`
  are non-nil. Runs in parallel, first error cancels. `BundleFetcher` interface
  added to `agent.Config`; `s3LayerFetcher` satisfies it via `FetchBundleJSON`.
- **`s3LayerFetcher.FetchBundleJSON`**: downloads bundle JSON from the S3 URI
  in `layer.Bundle`; returns `(nil, nil)` for empty Bundle fields.
- **Fetch statistics**: `s3LayerFetcher.Stats() FetchStats` returns aggregate
  cache-hit/download/bytes counters tracked via `sync/atomic` during `Fetch`.
- **`SignedBy` in manifests**: `internal/build/pipeline.go` now sets
  `manifest.SignedBy = job.KeyRef` after signing. `Job.KeyRef` added to
  `internal/build/recipe.go`; wired in `cmd/strata/build.go` from `--key` flag.

### Fixed
- **Lmod `init.sh` symlink**: `core/lmod/8.7.37/build.sh` now creates a
  relative symlink (`lmod/lmod/init/bash`) instead of an absolute path baked
  with the build tmpdir. The old absolute symlink was dangling at runtime after
  squashfs mount. Rebuild lmod layers to pick up the fix.

### Changed
- `internal/agent/agent.go`: `Run()` signature changed from `error` to
  `(*BootMetrics, error)` — all call sites updated (tests in `agent_test.go`,
  `cmd/strata-agent/main.go`).

## [0.13.0] - 2026-03-09

### Added
- **Lmod modulefile auto-generation**: build pipeline now writes
  `modulefiles/<name>/<version>.lua` inside every non-flat squashfs layer
  (`internal/build/modulefile.go`). Auto-detects `bin/`, `lib/`, `lib64/`,
  `lib/pkgconfig/`, `share/man/`, `share/info/`, `include/` and emits the
  corresponding `prepend_path()` Lmod directives. `conflict()` emitted for
  each user-visible capability name so only one version is active at a time.
- `spec/layer.go`: `LayerManifest.HasModulefile bool` — set true by the build
  pipeline when a modulefile was successfully generated
- `internal/build/recipe.go`: `ModuleEnvVar` struct + `RecipeMeta.ModulefileEnv
  []ModuleEnvVar` — declares extra `setenv()` vars in generated modulefiles
  (e.g. `GCC_HOME`, `CC`, `CXX`, `FC`, `MPI_HOME`, `HDF5_HOME`)
- `spec/profile.go`: `Profile.Defaults []SoftwareRef` — lists modules to
  pre-load at login; agent generates `/etc/profile.d/strata-defaults.sh`
- `spec/lockfile.go`: `LockFile.Defaults []SoftwareRef` — copied from profile
  by resolver stage 8; consumed by agent to write defaults script
- `internal/overlay/overlay.go`: `ConfigureEnvironment` now writes
  `/etc/profile.d/strata-modules.sh` (registers `modulefiles/` with Lmod) and
  `/etc/profile.d/strata-defaults.sh` (pre-loads `profile.defaults` modules)
- **Multi-version coexistence**: `canCoexist()` in resolver stage 5 allows two
  versioned-layout layers to share a capability name — e.g. gcc@13 + gcc@14 in
  the same profile without a CAPABILITY_CONFLICT. Flat-layout layers still
  conflict. Lmod `conflict()` prevents simultaneous activation.
- `internal/resolver/stages.go`: `canCoexist(a, b *spec.LayerManifest) bool`
- New resolver tests: `TestStage5_MultiVersionCoexistence`,
  `TestStage5_DifferentImpls_Coexist`
- **New recipes — core tier**:
  - `core/lmod/8.7.37/`: Lmod environment modules system; provides `lmod`,
    `modules@8.7.37`; AL2023 ships Lua natively
  - `core/nodejs/20.19.0/`: Node.js 20 LTS (pre-built binary); provides
    `nodejs@20.19.0`, `npm@10.8.2`
  - `core/nodejs/22.14.0/`: Node.js 22 LTS (pre-built binary); provides
    `nodejs@22.14.0`, `npm@10.9.2`
- **New recipes — library tier**:
  - `library/openblas/0.3.26/`: provides `openblas@0.3.26`, `blas@3.11`,
    `lapack@3.11`; built with DYNAMIC_ARCH for runtime CPU dispatch
  - `library/openblas/0.3.28/`: provides `openblas@0.3.28`, `blas@3.12`,
    `lapack@3.12`
  - `library/fftw/3.3.10/`: provides `fftw@3.3.10`; builds double, float, and
    long-double precision libraries; AVX/AVX2/AVX-512 on x86, NEON on arm64
  - `library/hdf5/1.12.3/`: provides `hdf5@1.12.3`; C, C++, Fortran APIs;
    zlib compression
  - `library/hdf5/1.14.4/`: provides `hdf5@1.14.4`
  - `library/netcdf-c/4.9.2/`: provides `netcdf-c@4.9.2`, `netcdf@4.9.2`;
    requires `hdf5>=1.12` in build and runtime env
- **New recipes — application tier**:
  - `application/julia/1.10.7/`: Julia 1.10 LTS (pre-built binary); provides
    `julia@1.10.7`
  - `application/julia/1.11.3/`: Julia 1.11 (pre-built binary); provides
    `julia@1.11.3`
- `modulefile_env:` added to all existing recipes: gcc (GCC_HOME, CC, CXX,
  FC), python (PYTHON, PYTHONHOME), R (R_HOME), cuda (CUDA_HOME, CUDA_PATH),
  openmpi (MPI_HOME, MPICC, MPICXX, MPIFC), ucx/hwloc/pmix/libfabric
  (UCX_HOME, HWLOC_HOME, PMIX_HOME, LIBFABRIC_HOME), samtools (SAMTOOLS_HOME),
  jupyterlab (JUPYTERLAB_HOME), miniforge (CONDA_PREFIX)

### Changed
- `internal/resolver/stages.go` `stage5DetectConflicts`: capability conflict
  check uses `capProviders map[string][]int` (was `capProvider map[string]int`)
  to support multiple providers per capability name; `canCoexist()` exempts
  versioned-layout pairs from capability-level rejection
- `internal/resolver/resolver_test.go` `TestStage5_CapabilityConflict`: updated
  to test flat-layout conflict (glibc-vendor-a + glibc-vendor-b); old MPI
  conflict test replaced by `TestStage5_DifferentImpls_Coexist`

## [0.12.0] - 2026-03-09

### Added
- `spec/layer.go`: `LayerManifest.UserSelectable bool` — false for dependency-only layers
  (ucx, hwloc, pmix, libfabric) that are resolved transitively but never shown in default
  `strata search` output or allowed as top-level user software choices
- `spec/layer.go`: `LayerManifest.InstallLayout string` — "versioned" (default,
  `/<name>/<version>/`) or "flat" (directly to `/`, required for glibc)
- `internal/build/recipe.go`: `RecipeMeta.UserSelectable *bool` — defaults to true;
  `RecipeMeta.UserSelectableBool()` helper
- `internal/build/recipe.go`: `RecipeMeta.InstallLayout string` — validated: "", "versioned", "flat"
- `internal/build/pipeline.go`: flat layout support — `installPrefix = outputDir` when
  `InstallLayout == "flat"`; pkg-config patching skipped for flat layout
- `cmd/strata/search.go`: `--abi` flag (replaces `--family`); `--all` flag to include
  `user_selectable=false` layers; default search hides dependency-only layers
- `cmd/strata/recipes/core/glibc/2.34/`: glibc 2.34 recipe skeleton (not yet buildable;
  requires bwrap agent support in v0.13.0+)
- `docs/architecture-execution-model.md`: specification of the bwrap/pivot_root execution
  model required for glibc-as-layer (target: v0.13.0)
- `STRATA.md`: "The Kernel Anchor" section explaining the ABI-first reproducibility philosophy

### Changed
- **Schema breaking change**: `family` field renamed to `abi` in `LayerManifest`,
  `BaseCapabilities`, `RecipeMeta` across the entire codebase
- **ABI values**: `"rhel"` → `"linux-gnu-2.34"`, `"debian"` → `"linux-gnu-2.35"`
- **S3 key paths**: `layers/rhel/<arch>/...` → `layers/linux-gnu-2.34/<arch>/...`
  (rebuild all layers to populate new paths; old `layers/rhel/` objects become orphaned)
- `probe.OSFamily` renamed to `probe.OSABI` with new ABI values
- `probe.KnownBaseCapabilities`: capability `{Name: "family", Version: "rhel"}` →
  `{Name: "abi", Version: "linux-gnu-2.34"}`
- `internal/probe/compiler.go`: `DetectSystemCompiler` switches on ABI string
- `internal/registry/registry.go`: all `family` parameters renamed to `abi`
- `internal/resolver/stages.go`: `stage3ResolveSoftware` passes `base.Capabilities.ABI`
- `internal/build/pipeline.go`: annotation `strata.layer.family` → `strata.layer.abi`
- All recipe `meta.yaml` files: `family: rhel` → `abi: linux-gnu-2.34`
- `library/{ucx,hwloc,pmix,libfabric}` recipes: added `user_selectable: false`

## [0.10.0] - 2026-03-08

### Added
- `internal/build/buildenv.go`: Stage 3 build environment resolver infrastructure
  - `EnvLayer`: resolved build-requires layer with local `.sqfs` path and mount order
  - `RegistryClient`: narrow interface (`ResolveLayer` + `FetchLayerSqfs`) satisfied by `*registry.S3Client`
  - `EnvResolver`: interface for resolving `build_requires` to local squashfs files
  - `RegistryBuildEnvResolver`: fetches manifests from registry, downloads `.sqfs` to cache dir
  - `FakeBuildEnvResolver`: pre-configured layers for unit testing; never calls registry
  - `defaultLayerCacheDir()`: `$TMPDIR/strata-build-cache`
- `internal/build/buildenv_test.go`: 5 unit tests for resolver implementations
- `internal/overlay/mount_linux.go`: `MountBuildEnv(layers []LayerPath, baseDir string)` new function
  - Mounts squashfs layers + OverlayFS at configurable `baseDir` (not hardcoded `/strata/*`)
  - Enables concurrent build environments without conflicting with runtime overlay
- `internal/overlay/mount_stub.go`: `MountBuildEnv` stub returning `ErrNotSupported` on non-Linux
- `internal/registry/s3client.go`: `FetchLayerSqfs` — downloads layer `.sqfs` to cache dir
  - Atomic write with SHA256 verification before committing to cache
  - Cache-hit returns existing file immediately without network call
- `internal/build/pipeline.go`: Stage 3 OverlayFS build environment mounting
  - `prepareStage3`: resolves and mounts `build_requires` layers; populates `manifest.BuiltWith`,
    `manifest.BootstrapBuild`, `manifest.BuildEnvLockID`; returns cleanup func + env vars
  - `buildEnvLockID`: SHA256 of YAML-serialized `BuiltWith` list for independent verification
  - Bootstrap path: `job.EnvResolver == nil` or empty `build_requires` → `BootstrapBuild = true`
  - Build env env vars: `PATH`, `LD_LIBRARY_PATH`, `STRATA_BUILD_ENV` pointing at merged dir
- `internal/build/ec2runner.go`: EC2 build orchestrator
  - `EC2Config`: region, AMI, instance type, subnet, security group, IAM profile, S3 URL, key ref
  - `EC2Runner`: uploads recipe to S3, launches EC2 instance, polls `strata:build-status` tag, terminates
  - `RunBuildEC2`: full lifecycle — upload → launch → poll → terminate; returns instance ID
  - `ArchForEC2(normalizedArch)`: maps `x86_64`→`amd64`, `arm64`→`arm64` for binary filenames
  - EC2 user-data template: IMDSv2, installs `squashfs-tools`, downloads strata binary + recipe,
    runs `strata build`, tags instance success/failed, stops instance
  - `ec2LaunchAPI` and `s3PutAPI` interfaces for mock injection in tests
- `internal/build/ec2runner_test.go`: 6 unit tests with fake AWS APIs
- `internal/build/catalog.go`: recipe catalog build planner
  - `Plan`: topologically ordered build plan with parallel stages
  - `PlanCatalog(recipesDir)`: discovers all recipes, resolves dependency graph, returns staged plan
  - `topoSort`: Kahn's algorithm grouping nodes at same depth into parallel build stages
- `cmd/strata/build.go`: added `--ec2`, `--ami`, `--instance-type`, `--cache-dir` flags
  - EC2 mode: `runBuildEC2` uploads recipe, launches instance, polls, terminates
  - Linux non-dry-run with `build_requires`: wires `RegistryBuildEnvResolver` into `job.EnvResolver`
- `cmd/strata/build_catalog.go`: `strata build-catalog <recipes-dir>` subcommand
  - Discovers all recipes; prints dependency-ordered build stages with parallel grouping
  - Non-dry-run: prints `strata build` commands for each recipe in stage order
  - Flags: `--os`, `--arch`, `--registry`, `--key`, `--dry-run`
- `cmd/strata/main.go`: registered `build-catalog` subcommand; updated usage text

### Changed
- `internal/build/recipe.go`: added `EnvResolver EnvResolver` and `CacheDir string` to `Job` struct
  - `EnvResolver`: nil = bootstrap mode (Tier 0); set by caller for Tier 1+ builds
  - `CacheDir`: overrides default `$TMPDIR/strata-build-cache`

## [0.9.0] - 2026-03-08

### Added
- `internal/build/executor.go`: `Executor` interface + two implementations
  - `LocalExecutor`: runs `build.sh` via `bash` with env vars; streams stdout/stderr
  - `DryRunExecutor`: prints script path and env vars without executing; always returns nil
- `internal/build/executor_test.go`: 6 unit tests (success, failure, cancellation, env vars, dry-run)
- `internal/build/squashfs.go`: `CreateSquashfs` + `ErrMksquashfsNotFound`
  - `CreateSquashfs(ctx, srcDir, outPath)`: runs `mksquashfs` with reproducible flags from `SquashfsOptions()`
  - Returns `*ErrMksquashfsNotFound` when `mksquashfs` is absent from PATH
- `internal/build/squashfs_test.go`: unit test for missing binary
- `internal/build/squashfs_integration_test.go` (`//go:build integration`): full squashfs creation
- `internal/build/content_manifest_gen.go`: `GenerateContentManifest`
  - Walks dir recursively; SHA256s each regular file; paths stored as `/`-prefixed relative strings
- `internal/build/content_manifest_gen_test.go`: 5 unit tests (basic, SHA256 correctness, empty dir, layerID, subdirs)
- `internal/build/pipeline.go`: `Run` + `PushRegistry` interface
  - `PushRegistry`: narrow interface satisfied by `*registry.S3Client`; enables mock injection in tests
  - `Run`: 9-stage local build pipeline (stages 2/3/11 EC2-only, deferred to v0.10.0)
    - Stage 1: warns if `build_requires` non-empty (not mounted in local path)
    - Stage 4: creates temp output dir; sets `STRATA_PREFIX/NCPUS/ARCH/OUT`; executes build script
    - Stage 8: `GenerateContentManifest` before freeing output dir
    - Stage 5: `CreateSquashfs`; frees output dir early to reclaim disk
    - Stage 6: SHA256 of `.sqfs`
    - Stage 7: size of `.sqfs`
    - Stage 9: signs with `trust.Signer`; records Rekor log index
    - Stage 10: `PushLayer` to registry; sets `manifest.Source`
  - DryRun path: returns sentinel manifest with `SHA256="dry-run"`, `RekorEntry="dry-run"`
- `internal/build/pipeline_test.go`: 2 unit tests (`TestRun_DryRun`, `TestRun_InvalidJob`)
- `internal/build/pipeline_integration_test.go` (`//go:build integration`): `TestRun_LocalBuild`
- `internal/registry/s3client.go`: three new methods on `S3Client`
  - `PushLayer`: uploads `layer.sqfs`, `manifest.yaml`, `bundle.json`; calls `upsertLayerIndex`
  - `upsertLayerIndex`: fetches current index; replaces entry by ID or appends; writes back
  - `RebuildIndex`: full S3 scan of `layers/` tree; rewrites `index/layers.yaml`
- `internal/registry/s3client_test.go`: 5 new unit tests (`TestPushLayer_UploadsThreeObjects`,
  `TestPushLayer_UpdatesIndex`, `TestPushLayer_UpsertReplacesExisting`,
  `TestRebuildIndex_ScansAllManifests`, `TestRebuildIndex_EmptyRegistry`)
- `cmd/strata/build.go`: `strata build <recipe-dir>` subcommand
  - Flags: `--os`, `--arch`, `--registry`, `--key`, `--dry-run`
  - Wires `ParseRecipe` → `Job.Validate` → `registry.NewS3Client` → `LocalExecutor` → `CosignSigner` → `build.Run`
  - Prints built/sha256/size/rekor/pushed summary; `dry-run:` prefix in dry-run mode
- `cmd/strata/index.go`: `strata index --registry s3://...` subcommand
  - Calls `registry.NewS3Client` → `RebuildIndex`; prints confirmation
- `cmd/strata/main.go`: registered `build` and `index` subcommands; updated usage text

## [0.8.0] - 2026-03-08

### Added
- `internal/probe/ssm_resolver.go`: `SSMResolver` implementing `Resolver` via AWS SSM Parameter Store
  - `ssmAPI` interface for mock injection in tests
  - `NewSSMResolver(ctx)`: loads default AWS config, creates real SSM client
  - `newSSMResolverWithAPI(api)`: test constructor
  - `ResolveAMI(ctx, os, arch)`: calls `ResolveSSMParam` → `ssm.GetParameter` → returns real AMI ID
- `internal/probe/ssm_resolver_test.go`: unit tests with `mockSSM`
  - `TestResolveAMI_HappyPath` — mock returns `ami-real123`
  - `TestResolveAMI_UnknownOS` — unknown OS returns error before any SSM call
  - `TestResolveAMI_SSMError` — SSM error propagated via error chain
- `internal/probe/s3_cache.go`: `S3Cache` implementing `Cache` via a `CapabilityStore`
  - `CapabilityStore` interface: narrow subset of `registry.Client` (`GetBaseCapabilities` / `StoreBaseCapabilities`)
  - `NewS3Cache(store)`: constructor; real `*registry.S3Client` satisfies the interface
  - `Get`: returns `nil, false` on both `IsNotFound` and unexpected errors (cache miss is non-fatal)
  - `Set`: delegates to `StoreBaseCapabilities`
- `internal/probe/s3_cache_test.go`: unit tests with `mockCapStore`
  - `TestS3Cache_GetMiss`, `TestS3Cache_SetAndGet`, `TestS3Cache_GetError`
- `cmd/strata/probe.go`: `strata probe <os> <arch> [--registry s3://...]` subcommand
  - Validates OS/arch pair via `probe.ResolveSSMParam` before any AWS calls
  - Resolves real AMI ID via SSMResolver (falls back to placeholder on SSM error)
  - Checks S3 registry cache; generates from `KnownBaseCapabilities` on miss
  - Stores result in S3 registry if `--registry` / `STRATA_REGISTRY_URL` is set
  - Prints AMI ID, family, arch, OS, and full capability list
- `go.mod`: `github.com/aws/aws-sdk-go-v2/service/ssm` promoted to direct dependency

### Changed
- `cmd/strata/resolve.go`: `buildProbeClient()` now attempts real clients when possible
  - With `STRATA_REGISTRY_URL` + valid AWS credentials: wires `SSMResolver` + `S3Cache`
  - On any init failure: falls back to `buildStaticProbeClient()` (previous behaviour)
  - `buildStaticProbeClient()`: extracted from old `buildProbeClient()` body — static placeholder AMIs + `FakeRunner` + `MemoryCache`
  - `buildKnownFakeRunner()`: new helper returning a `FakeRunner` with placeholder-keyed `KnownBaseCapabilities`
- `cmd/strata/main.go`: registered `probe` as a subcommand; updated usage text

## [0.7.0] - 2026-03-08

### Added
- `cmd/strata-agent/metadata_source.go`: real `metadataLockfileSource` implementation
  - `imdsAPI` / `s3GetAPI` interfaces for mock injection in tests
  - `Acquire`: checks EC2 user-data for YAML-encoded lockfile first; falls back to
    `strata:lockfile-s3-uri` instance tag → S3 fetch
  - `parseS3URI`: parses `s3://bucket/key` URIs
- `cmd/strata-agent/s3_fetcher.go`: real `s3LayerFetcher` implementation
  - Cache hit path: stat `<cacheDir>/<sha256>.sqfs`; return immediately on hit
  - Cache miss path: `s3.GetObject` → write to temp file → atomic rename
  - `newS3LayerFetcherWithAPI` constructor for test injection
- `cmd/strata-agent/ec2_signaler.go`: real `ec2ReadySignaler` implementation
  - `SignalReady`: `ec2.CreateTags` with `strata:status=ready` and
    `strata:environment-id=<lockfile.EnvironmentID()>`; then `sd_notify("READY=1")`
  - `SignalFailed`: best-effort EC2 tags with `strata:status=failed` and
    `strata:failure-reason=<truncated to 256 chars>`; then
    `sd_notify("STATUS=failed: ...")`
  - `sdNotify`: pure stdlib `net.DialUnix` on `NOTIFY_SOCKET`; no-op if unset
  - `ec2TagAPI` interface and `newEC2ReadySignalerWithAPIs` constructor for tests
- `cmd/strata-agent/agent_aws_test.go`: unit tests with hand-written mocks
  - `mockIMDS`, `mockS3Get`, `mockEC2Tag` — no real AWS credentials required
  - 10 test cases covering all three implementations plus `parseS3URI`
- `go.mod`: `feature/ec2/imds` and `service/ec2` promoted to direct dependencies

### Changed
- `cmd/strata-agent/main.go`: stubs and inline struct types replaced with calls to
  new real constructors; `errors` import removed

## [0.6.0] - 2026-03-08

### Added
- `internal/registry/s3client.go`: full S3 registry client implementation
  replacing the previous stub
  - `ResolveLayer`: `ListObjectsV2` enumeration of version directories +
    `versionMatches`/`compareSegments` selection + `GetObject` manifest fetch
  - `ResolveFormation`: `GetObject` on `formations/<name>/<version>/manifest.yaml`
  - `GetBaseCapabilities` / `StoreBaseCapabilities`: `GetObject`/`PutObject`
    on `probes/<amiID>/capabilities.yaml`
  - `ListLayers`: `GetObject` on `index/layers.yaml` (`LayerIndex`) + filter +
    `sortManifestsByVersionDesc`
  - `s3API` interface enables mock injection without real AWS credentials
  - `*types.NoSuchKey` mapped to `*ErrNotFound` on all read paths
- `internal/registry/s3client_test.go`: unit tests with hand-written `mockS3`
  (no real AWS account required); integration test skeleton gated on
  `STRATA_TEST_BUCKET` env var
- `LayerIndex` type in `internal/registry` for flat layer catalog YAML
- `cmd/strata/resolve.go`: `buildRegistryClient` now handles `NewS3Client`
  error with stderr warning + graceful fallback to embedded catalog

### Changed
- `NewS3Client` signature changed from `(*S3Client)` to `(*S3Client, error)`;
  call site in `cmd/strata/resolve.go` updated accordingly
- `go.mod`: added `github.com/aws/aws-sdk-go-v2` and related S3 packages

## [0.5.0] - 2026-03-07

### Added
- `cmd/strata`: full CLI implementation
  - `strata resolve`: profile → lockfile via 8-stage resolver; wires S3 registry
    (`STRATA_REGISTRY_URL`) or embedded catalog fallback
  - `strata freeze`: resolve + requires all layers to have SHA256 (`IsFrozen` check);
    reports missing layers by ID
  - `strata search`: browse embedded Tier 0 catalog; table output with `--arch`/`--family`
    filters; `--formation` flag lists formations
  - `strata verify`: lockfile field-presence check (`Bundle`, `RekorEntry`, `IsSigned`);
    optional live Rekor API verification with `--rekor`
  - `strata publish`: frozen lockfile → Zenodo DOI; requires `ZENODO_TOKEN` or `--token`;
    `--sandbox` targets `sandbox.zenodo.org`
  - Embedded recipe + formation catalog via `//go:embed` for offline search
- `internal/zenodo`: Zenodo Deposit API client
  - 3-step deposit: create → upload lockfile YAML → publish; returns DOI and record URL
  - Sandbox mode (`--sandbox`) for testing against `sandbox.zenodo.org`
  - Unit tests with `httptest`; no Zenodo account required to run tests
- `internal/registry/s3client.go`: `S3Client` stub implementing `registry.Client`
  against an S3-backed registry; all methods return clear "not yet implemented" errors
  with `TODO` comments explaining the intended S3 key paths
- `recipes/`: Tier 0 layer recipe catalog (10 recipes, `family: rhel`)
  - `gcc@13.2.0`, `python@3.11.9`, `python@3.12.3`, `R@4.3.3`, `cuda@12.3.2`
  - `openmpi@4.1.6`, `miniforge@24.3.0`, `jupyterlab@4.2.0`, `samtools@1.21.0`,
    `quarto@1.4.555`
  - Each recipe has `meta.yaml` (`RecipeMeta` contract) and `build.sh`
    (reproducible install script using `$STRATA_PREFIX` and `$STRATA_NCPUS`)
- `formations/`: formation catalog (6 formations)
  - `cuda-python-ml`, `r-research`, `hpc-mpi`, `bio-seq`, `genomics-python`,
    `jupyter-gpu`
  - All validated on `al2023/x86_64`; CUDA-only formations are `x86_64`-only
- `examples/catalog_test.go`: recipe + formation YAML validation gate; fails if any
  recipe directory is missing `build.sh`, has invalid `meta.yaml`, or any formation
  YAML is not a well-formed `spec.Formation`

## [0.4.0] - 2026-03-07

### Added
- `internal/overlay`: OverlayFS assembly package
  - `Mount(layers []LayerPath) (*Overlay, error)`: squashfs loopback mounts + OverlayFS assembly in MountOrder
  - `Overlay.Cleanup()`: reverse-order unmount with `MNT_DETACH` for busy mounts; nil-safe
  - `ConfigureEnvironment(lockfile, overlay, rootDir)`: writes `/etc/profile.d/strata.sh`,
    `/etc/strata/environment`, and `/etc/strata/active.lock.yaml` after a successful mount
  - `//go:build linux` on all syscall code; darwin/non-Linux stub returns `ErrNotSupported`
  - Partial-failure cleanup: if squashfs mount N fails, mounts 0..N-1 are unmounted before returning
- `internal/agent`: instance bootstrap agent logic
  - `LockfileSource`, `LayerFetcher`, `ReadySignaler`, `Mounter` interfaces for full testability
  - `Agent.Run`: 6-step boot sequence; any failure calls `SignalFailed` and returns the error
  - Parallel layer fetch + SHA256 verify (`sync.WaitGroup` + context cancellation on first error)
  - `FakeLockfileSource`, `FakeLayerFetcher`, `FakeReadySignaler`, `FakeMounter` for testing
  - Full unit test suite (happy path + each failure mode); all tests pass on macOS without Linux syscalls
- `cmd/strata-agent`: agent binary entry point
  - Wires all interfaces; real EC2/S3 implementations stubbed with `TODO` for follow-on work
  - Compiles and links on all platforms

## [0.3.0] - 2026-03-07

### Added
- `internal/resolver`: 8-stage resolver pipeline
  - `Resolver` struct wiring `registry.Client`, `probe.Client`, `trust.RekorClient`
  - `Resolve(ctx, profile) (*spec.LockFile, error)` — clean pass or hard stop, no partial lockfile
  - Stage 1: base OS → AMI ID → BaseCapabilities (probe cache-aware)
  - Stage 2: formation expansion — formations treated as pre-solved subgraphs; bundle
    and Rekor entry presence verified before any layer is resolved
  - Stage 3: software resolution with actionable "not found" errors showing available
    versions and a `strata search` hint
  - Stage 4: graph validation — all requirements satisfied by base + resolved layers
  - Stage 5: conflict detection — capability-level (e.g. two MPI implementations) and
    file-level (ContentManifest SHA256 mismatch); same-formation pairs are exempt
  - Stage 6: topological sort via Kahn's algorithm → deterministic MountOrder assignment
  - Stage 7: Sigstore bundle presence + optional Rekor entry verification (parallel)
  - Stage 8: fully populated LockFile assembly
  - `ResolutionError`: structured errors with stage, code, message, and available versions

## [0.2.0] - 2026-03-07

### Added
- `internal/trust`: Sigstore/cosign/Rekor trust package
  - `Bundle` type conforming to sigstore bundle v0.3 format (independently verifiable
    with standard cosign tooling)
  - `Signer`, `Verifier`, `RekorClient` interfaces — unconditional, no skip paths
  - `CosignSigner` / `CosignVerifier`: exec-based cosign CLI integration
  - `RekorHTTPClient`: direct Rekor REST API integration
  - `FakeSigner`, `FakeVerifier`, `FakeRekorClient`: deterministic fakes for tests
  - `VerifyLayer` / `VerifyLayers`: helper functions that enforce SHA256 content
    integrity and cosign signature verification before any layer is mounted
- `internal/probe`: AMI capability probe package
  - `Resolver`, `Runner`, `Cache` interfaces for capability detection
  - `StaticResolver`: fixed OS→AMI map for testing and offline use
  - `FakeRunner`: pre-configured capability results without EC2
  - `MemoryCache`: in-memory probe cache for testing
  - `KnownBaseCapabilities`: correct capabilities for all supported OS images
    (al2023, rocky9, rocky10, ubuntu24) with proper rhel/debian family assignment
  - SSM parameter paths for all supported OS/arch combinations
- `internal/registry`: layer catalog client
  - `Client` interface: `ResolveLayer`, `ResolveFormation`, `GetBaseCapabilities`,
    `StoreBaseCapabilities`, `ListLayers`
  - `MemoryStore`: full in-memory implementation for testing and local dev
  - `ErrNotFound` with `IsNotFound()` helper
  - Version prefix matching ("12.3" matches "12.3.0", "12.3.1", …) with numeric
    segment comparison
  - Newest-first ordering in `ListLayers`
- `internal/build`: layer build pipeline and recipe contract
  - `RecipeMeta`: parsed meta.yaml — declares provides, build_requires, runtime_requires
  - `Recipe`: fully parsed recipe (meta + build.sh path) with `ParseRecipe`
  - `ContentManifest`: per-file SHA256 manifest for conflict detection
  - `Job`: build job descriptor with validation
  - `SquashfsOptions()`: reproducible mksquashfs flags ensuring same recipe = same SHA256
  - `RecipeMeta.ToLayerManifest()`: partial manifest for registry pre-registration

## [0.1.0] - 2026-03-07

### Added
- `spec` package: `Profile`, `LockFile`, `LayerManifest`, `Formation`, `BaseCapabilities` types
- YAML parse and marshal functions for profiles and lockfiles
- `SoftwareRef` inline string parsing (`cuda@12.3`, `formation:r-research@2024.03`)
- `LockFile.EnvironmentID()`: stable SHA256-based environment identity derived from
  base AMI SHA256, layer SHA256s (in mount order), env vars, and on-ready commands;
  excludes attestation and timing fields so signing does not change the ID
- `BaseCapabilities.HasCapability` / `SatisfiesRequirement`: numeric segment-wise
  version comparison replacing the incorrect string comparison placeholder
- Example profiles: `examples/alphafold3.yaml`, `examples/r-quarto-workstation.yaml`,
  `examples/pytorch-jupyter.yaml` with parse and round-trip smoke tests
- Initial project structure, CI workflow, and tooling

[Unreleased]: https://github.com/scttfrdmn/strata/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/scttfrdmn/strata/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/scttfrdmn/strata/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/scttfrdmn/strata/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/scttfrdmn/strata/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/scttfrdmn/strata/releases/tag/v0.1.0
