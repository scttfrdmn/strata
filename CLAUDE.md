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

## AWS Infrastructure

All EC2 build instances, S3 buckets, and IAM roles live in the **Strata Infrastructure**
account (400563159792), which is a sub-account under scttfrdmn's management account.

**Always use `--profile strata` for AWS CLI commands in this project.**

The `strata` profile (in `~/.aws/config`) assumes `OrganizationAccountAccessRole` in account
400563159792 using `scttfrdmn` as the source profile. Region: `us-east-1`.

```sh
aws --profile strata ec2 describe-instances   # correct
aws ec2 describe-instances                    # wrong — hits management account, finds nothing
```

Key resources:
- S3 registry bucket: `strata-registry`
- IAM instance profile: `strata-builder`
- Builder instances use SSM Session Manager (not SSH — 1Password agent causes hangs)

## Versioning

Semantic Versioning 2.0.0. [CHANGELOG.md](CHANGELOG.md) follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Milestones on GitHub track progress toward each version.
