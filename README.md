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

## Development

```sh
make test     # test with race detector and coverage
make lint     # golangci-lint
make check    # vet + lint + test
make build    # build ./cmd/strata
```

## License

Apache License 2.0 — Copyright 2026 Scott Friedman
