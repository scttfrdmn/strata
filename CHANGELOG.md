# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
