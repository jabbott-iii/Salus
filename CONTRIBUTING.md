# Contributing to Salus

Thanks for your interest in improving Salus. This guide covers how to propose
changes, set up a development environment, validate your work, and open a pull
request. Architecture and maintainability rules are defined in
[`intel/maint.md`](intel/maint.md), which takes precedence if this guide and
that document disagree. Go-specific rules are in
[`intel/golang.md`](intel/golang.md).

Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Contribution workflow

1. **Open an issue first.** Create an issue to pitch an addition or change.
   Pull requests with no corresponding issue will be declined.
2. **Comment on existing issues.** If an issue for the item already exists,
   comment on it before opening a pull request that addresses it.
3. **Wait for confirmation.** A maintainer must confirm the pitched concept on
   the issue before you open a pull request that modifies the code base.
4. **Branch and implement.** Keep the change scoped to the confirmed issue.
5. **Validate locally** (see [Validation](#validation)), then open a pull
   request that links the issue.

## Development setup

### Prerequisites

- **Go 1.26** or newer, matching the `go` directive in `go.mod`.
- **A C toolchain** (for example `gcc` or `clang`) with `CGO_ENABLED=1`.
  Salus uses `gorm.io/driver/sqlite`, which depends on the CGO driver
  `github.com/mattn/go-sqlite3`. A build with CGO disabled compiles but
  cannot open its database at runtime.
- **Git.**
- Optional: **golangci-lint v2.13.2**, the version CI uses.
- Optional: **Docker**, to build the container image.

A dev container definition is provided in `.devcontainer/devcontainer.json`
(Ubuntu base with the Go and Docker-outside-of-Docker features). Confirm that a
C compiler is available inside it (`gcc --version`) before building.

### Build and run

From the repository root:

```bash
go mod download
CGO_ENABLED=1 go build -o salus .
./salus check list
./salus check run
```

By default Salus stores its database in a per-user data directory (see
Configuration in `README.md`). To keep development data separate, set
`SALUS_DB_PATH`:

```bash
SALUS_DB_PATH="$(mktemp -d)/salus.db" ./salus check run --only misconfig
```

## Validation

Run these from the repository root before opening a pull request. They mirror
the checks in `.github/workflows/ci.yml`.

```bash
gofmt -s -w .                      # required before every pull request
go vet ./...
go test ./...
go test -race ./...                # required for concurrency-related changes
golangci-lint run                  # CI uses golangci-lint v2.13.2
GOOS=darwin golangci-lint run ./...   # CI also lints on macOS
GOOS=windows golangci-lint run ./...  # ...and on Windows
go mod tidy && git diff --exit-code   # CI fails if go.mod/go.sum drift
```

CI also runs tests on Linux, macOS, and Windows, and CodeQL and gosec on every
push and pull request. Make sure CI passes on your pull request.

## Coding expectations

- **License header.** Maintain the Apache-2.0 license header on every source
  file. Copy it from an existing `.go` file. Build constraints
  (`//go:build ...`) go above the header.
- **Formatting.** Run `gofmt -s -w .` at the repository root before opening a
  pull request.
- **Scope.** Keep changes narrowly scoped. Avoid unrelated refactoring,
  formatting churn, renames, or dependency upgrades.
- **Compatibility.** Command names, flags, exit codes (`0` PASS, `1` WARN,
  `2` FAIL), check keys, JSON output fields, `SALUS_DB_PATH`, and the database
  schema are public contracts. Do not change them without explicit approval
  on the issue.
- **Errors.** Wrap errors with context (`fmt.Errorf("...: %w", err)`), and
  check and return every write to a command's output writer.
- **External commands.** Use `exec.CommandContext` with a timeout and separate
  arguments. Never invoke a shell, and validate user-supplied arguments.
- **Dependencies.** Prefer the standard library and existing dependencies.
  Explain any new dependency in the issue, and update `NOTICE` when the module
  graph changes.
- **New checks** follow the checklist in
  [`intel/maint.md`](intel/maint.md#adding-a-new-check-checklist): key
  constant, registry entry, seeded catalog entry, tests, and documentation.

## Tests

- Add or update tests for every behavior change. Use table-driven tests where
  inputs share a pattern.
- Tests must be deterministic. New tests must not depend on Docker,
  Kubernetes, systemd, or the host's current resource levels. Use fixtures or
  injected fakes instead.
- Use `t.TempDir()` for files and databases (see `newTestDatabase` in
  `internal/database_test.go`) and `t.Cleanup()` for teardown.

## Documentation

Update documentation in the same pull request when your change affects it:

- `README.md` for user-visible commands, flags, behavior, or installation.
- `intel/map.md` when files, components, dependencies, or data flows change.
- `intel/cybersec.md` when you find or fix a security issue. Never delete
  existing items.
- `intel/history.md` for significant changes. Append only.
- `intel/plan.md` and `intel/notes.md` when work items or open questions
  change.

See `AGENTS.md` for the full documentation rules.

## Pull request expectations

- Link the confirmed issue and describe what changed and why.
- List the validation commands you ran and their results.
- Include tests for the change, and note any checks you could not run.
- Call out security-sensitive changes explicitly for review.
- Do not include secrets, credentials, tokens, local databases (`*.db`), or
  `.env` files.
- Keep the pull request focused. Split unrelated work into separate issues.

## Security issues

Do not report vulnerabilities in public issues. Contact the code owner
(`@jabbott-iii`, see `CODEOWNERS`) privately. Known issues and remediation
status are tracked in [`intel/cybersec.md`](intel/cybersec.md).

## Releases

Releases are cut by maintainers. `make release VERSION=vX.Y.Z` creates and
pushes an annotated tag, which triggers the CD workflow.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0 (see [`LICENSE`](LICENSE), section 5).
