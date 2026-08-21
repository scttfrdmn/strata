# Strata

[![CI](https://github.com/scttfrdmn/strata/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/scttfrdmn/strata/actions/workflows/ci.yml)

Composable, reproducible, cryptographically attested compute environments for cloud-based research.

## Overview

Researchers declare what they want. The system composes, attests, and delivers it — reproducibly.

```yaml
name: r-quarto-workstation
base:
  os: al2023

software:
  - formation:r-research@2024.03
  - quarto@1.4
  - pandoc@3.1
  - texlive@2024
  - git@2.43

instance:
  type: r7i.2xlarge
```

The system guarantees R is installed, RStudio Server is running, every declared piece of software is present at the declared version, and the environment is identical every time this profile is resolved. See [STRATA.md](STRATA.md) for the full design.

## Status

Early development. The `spec` package (core types) is complete. Resolver, agent, registry client, and CLI are in progress — see [GitHub Issues](https://github.com/scttfrdmn/strata/issues).

## Requirements

- Go 1.22+

## Offline resolution (no AWS)

`STRATA_REGISTRY_URL` accepts a `file://` URL as well as `s3://`. A local
directory with the registry layout is then read like any other registry, with no
AWS credentials and no network access involved.

```sh
export STRATA_REGISTRY_URL=file:///var/strata-local
strata resolve profile.yaml -o profile.lock.yaml
strata freeze  profile.yaml -o profile.lock.yaml
```

`resolve`, `freeze`, `freeze-layer`, `fold`, `capture`, `scan`, and `stratify`
accept a `file://` registry. `build`, `index`, `probe`, and `remove` are still
S3-only and reject one.

The directory layout is the same as the S3 one:

```
/var/strata-local/
  index/layers.yaml                                        # layer catalog
  layers/<abi>/<arch>/<name>/<version>/manifest.yaml        # layer manifest
  layers/<abi>/<arch>/<name>/<version>/layer.sqfs           # layer content
  layers/<abi>/<arch>/<name>/<version>/bundle.json          # Sigstore bundle
  formations/<name>/<version>/manifest.yaml
  probes/<ami-id>/capabilities.yaml
  locks/<environment-id>.yaml
```

Note that the *embedded* Tier 0 catalog cannot resolve offline on its own. It is
built from the recipes in `cmd/strata/recipes/`, which carry no `sha256`,
`bundle`, or `rekor_entry` because nothing has been built from them yet — so
resolution against it stops in stage 7 with `BUNDLE_MISSING`. Signed layers in a
registry, local or S3, are what stage 7 requires.

To resolve something offline right now, in a fresh clone, with no registry of
your own:

```sh
make offline-resolve
```

That materializes the test fixture registry under `bin/fixture/` and resolves a
profile through it. CI runs the same sequence on every pull request. The fixture
lives in `internal/testregistry/` and is reusable by any test that needs a
lockfile; its two deliberate limits (`layer.sqfs` is not a real squashfs image,
`bundle.json` is not a real signature) are documented in that package.

## Development

```sh
make test             # test with race detector and coverage
make lint             # golangci-lint
make check            # vet + lint + test
make build            # build ./cmd/strata
make offline-resolve  # prove a lockfile can be produced with no AWS credentials
```

## License

Apache License 2.0 — Copyright 2026 Scott Friedman
