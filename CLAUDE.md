# CLAUDE.md

## Project

Strata — composable, reproducible, cryptographically attested compute environments
for cloud-based research. Full design: [STRATA.md](STRATA.md).

All work is tracked in [GitHub Issues](https://github.com/scttfrdmn/strata/issues)
with labels and milestones. No standalone tracking files.

## Go Conventions

- Module: `github.com/scttfrdmn/strata` — Go 1.22
- Standard layout: `spec/`, `cmd/strata/`, `internal/<component>/`
- Idiomatic Go: exported types with godoc, no unnecessary abstractions
- A+ Go Report Card: `gofmt`, `go vet`, and `golangci-lint` must pass clean
- Tests: race detector always on (`-race`); meaningful coverage for all non-trivial logic
- No `init()` functions; no package-level globals

## Key Commands

```sh
make build   # build ./cmd/strata → bin/strata
make test    # test with race detector + coverage report
make lint    # golangci-lint
make check   # vet + lint + test
```

## Versioning

Semantic Versioning 2.0.0. [CHANGELOG.md](CHANGELOG.md) follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Milestones on GitHub track progress toward each version.
