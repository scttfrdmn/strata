# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
