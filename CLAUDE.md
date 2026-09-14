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

5. **Labels on every issue; milestone when it's scheduled.** Every issue needs at
   minimum one `component:` label and one `priority:` label. A **milestone is a
   release-planning decision, not a filing requirement**: assign it when the issue
   genuinely belongs to a release's scope. An unmilestoned issue is a valid backlog
   state, not a violation — do **not** bulk-assign milestones to drive the
   unmilestoned count to zero, which only makes every milestone unreadable. (This
   rule was narrowed after 42 of 60 open issues carried no milestone: a rule most
   of its population violates is failing its own audit, not describing a backlog.)

## Go Conventions

- Module: `github.com/scttfrdmn/strata` — Go 1.27 (the `go` directive in `go.mod`; the module does not build on anything older)
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

**Release-refresh gate — `PROPERTIES.md` must reflect the code being tagged, not
lag it.** `propgen` regenerates the Status column from the refutation register,
but the register *prose* is hand-authored and can outrun the code: v0.23.0
shipped with T1/T5 still describing the nil-verifier path as open, though #93
closed it in that same release (#172). So before `git tag` on any release:
1. Discharge the register rows for every issue the release closes, on
   **re-derived evidence** per §2.1 rule 11 — the citation is the re-run test at
   the RC head, not the closed issue.
2. Run `go run ./cmd/propgen` and confirm it reports `no drift`.
3. Confirm the §3 prose and the distribution header in the preamble match the
   RC (both are hand-authored; `propgen` does not touch them).
