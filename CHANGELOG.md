# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
