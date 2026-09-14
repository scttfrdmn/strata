# Strata

[![CI](https://github.com/scttfrdmn/strata/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/scttfrdmn/strata/actions/workflows/ci.yml)

Composable, reproducible, cryptographically attested compute environments for cloud-based research.

## Overview

Researchers declare what they want. The system composes, attests, and delivers it — reproducibly.

```yaml
name: r-quarto-workstation
base:
  os: al2023
  arch: x86_64

software:
  - formation: r-research@2026.03
  - quarto@1.4

instance:
  type: r7i.2xlarge
```

This profile resolves against the shipped catalog — a test extracts this exact block and resolves it on every run, so it cannot drift out of parseability. The goal: every declared piece of software is present at the declared version, and the resolved environment is identical every time the profile is resolved. See [STRATA.md](STRATA.md) for the full design.

## Status

Actively developed; latest release **v0.23.0** (see [CHANGELOG.md](CHANGELOG.md)). The breadth is real — resolver (8-stage pipeline), agent, S3/local/federated registry clients, and a 20-command CLI are all implemented, along with package resolution, `freeze`/`update`/`diff`, OCI export, layer capture and folding, environment scanning, and Zenodo publication.

What is implemented is not all equally *enforced*. See **Current trust guarantees** below for what is and is not cryptographically checked today — the honest version distinguishes shipped breadth from the trust work still open (tracked in [GitHub Issues](https://github.com/scttfrdmn/strata/issues), milestone *v0.25.0 — the trust chain is real*). The falsifiable-property register in [PROPERTIES.md](PROPERTIES.md) is the authoritative account of which invariants hold.

## Requirements

- Go 1.24+ (the `go` directive in `go.mod`)

## Current trust guarantees

What Strata cryptographically enforces **today**:

- **Content integrity.** Every layer mounted from the registry is hashed and checked against the digest recorded in the lockfile, on every use including cache hits (`strata run` and the agent).
- **The agent is fail-closed.** `strata-agent` refuses to boot rather than mount a layer it cannot verify the authenticity of; degrading to unverified requires the explicit `STRATA_AGENT_ALLOW_UNVERIFIED` opt-out (#93).
- **A layer whose contents contradict its manifest is refused at mount** (#146).

What is **not** yet enforced — do not rely on it as a trust boundary:

- **`strata run` and `strata verify` do not verify lockfile-level signatures** (#60). `strata verify` without `--rekor` is a presence check.
- **An offline resolve is not a trust decision.** Resolving against the embedded catalog with no registry configured accepts unsigned layers with a loud warning, so the shipped formations resolve for local use; the lockfile it produces is unsigned. Configure a registry to require signatures.
- **No freshness bound or set-level attestation yet** (#101): rollback and mix-and-match are not prevented.

This section is deliberately specific because the gap between the design in `STRATA.md` and what is enforced is the thing most worth being honest about.

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

The *embedded* Tier 0 catalog — built from the recipes in `cmd/strata/recipes/` —
carries no `sha256`, `bundle`, or `rekor_entry`, because nothing has been built
from them yet. Resolving against it with **no registry configured** therefore
accepts those unsigned layers with a loud warning that the resolve is not a trust
decision and the lockfile is not signed (so the shipped formations and examples
resolve for local use). The moment you configure a registry — `file://` or
`s3://` — stage 7 is strict again and requires a Sigstore bundle per layer.

To resolve a profile against a *signed* registry offline, in a fresh clone, with
no registry of your own:

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
