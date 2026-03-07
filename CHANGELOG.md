# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/scttfrdmn/strata/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/scttfrdmn/strata/releases/tag/v0.1.0
