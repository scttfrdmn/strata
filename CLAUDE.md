# CLAUDE.md

## Project

Strata — composable, reproducible, cryptographically attested compute environments
for cloud-based research. Full design: [STRATA.md](STRATA.md).

All work is tracked in [GitHub Issues](https://github.com/scttfrdmn/strata/issues)
with labels and milestones.

**GitHub project hygiene rules — follow these every session:**

1. **No standalone tracking files.** Never create `*_status.md`, `*_plan.md`,
   `*_todo.md`, `strata-chat.md`, or any ad-hoc planning/status documents in the repo.
   If it belongs in the project, open a GitHub Issue. If it belongs in docs, put it
   in `docs/`. Everything else gets deleted.

2. **Keep issues current.** When work on an issue begins, add a comment noting what
   was done. When work is complete, close the issue. Do not leave issues open for
   work that has already shipped.

3. **Keep milestones current.** When all issues in a milestone are closed and the
   version is tagged, close the milestone. Never leave a released version's milestone
   open.

4. **Open issues for new work.** Before starting non-trivial work that isn't tracked
   by an existing issue, create one — with the right milestone and labels — so the
   intent is visible before the implementation begins.

5. **Labels and milestones on every issue.** Every issue needs at minimum one
   `component:` label, one `priority:` label, and a milestone assignment.

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
