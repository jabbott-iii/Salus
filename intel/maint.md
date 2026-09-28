# Architecture and Maintainability Guidance

This document is the authoritative source for Salus architecture and
maintainability rules. `CONTRIBUTING.md` summarizes the contributor-facing
subset; if the two disagree, this document wins and `CONTRIBUTING.md` must be
corrected. Go language rules live in [`golang.md`](golang.md), which
`AGENTS.md` designates as the authoritative guidance on Go language usage. They
apply to all Go work in this repository.

Last reviewed: 2026-09-27 (against commit `753252e` plus uncommitted P2-3/P2-4
changes).

## 1. Purpose and scope

Salus is a single-binary Go CLI that runs local environment health checks
(disk, memory, CPU load, Docker, Kubernetes, service uptime, common
misconfigurations), prints a PASS/WARN/FAIL report, and records each run as a
job in a local SQLite database.

Salus is a command-line tool only. No TUI is planned (Q-001). It has no
network listener, no HTTP API, and no authentication layer. The
REST API rules in `AGENTS.md` do not currently apply; they become binding if an
HTTP interface is ever added.

## 2. Architecture overview

| Layer | Files | Responsibility |
|---|---|---|
| Entry point | `main.go`, `version.go` | `run()` builds the root command with the build `version` (`--version`) and a lazy, memoized database opener (`internal.DatabasePath` + `internal.OpenDatabase`, closed on return), executes it, and maps the result to an exit code. `main()` only calls `os.Exit(run(...))`. |
| CLI | `internal/logic-cli.go` | Cobra command tree (`check list`, `check run`, `jobs list`, `jobs show`) and flag parsing. `check run` returns `*ExitStatusError` for WARN/FAIL. |
| Checks | `internal/health.go`, `internal/health-thresholds.go`, `internal/health-resources_linux.go`, `internal/health-resources_other.go` | Check registry, thresholds, and the individual check functions. |
| Reporting | `internal/report.go` | Text and JSON rendering, worst-status aggregation, exit-code constants, `ExitStatusError`, and `ExitCode`. |
| Persistence | `internal/database.go`, `internal/database-path.go`, `internal/scan-store.go`, `internal/seed.go` | Database path resolution (`SALUS_DB_PATH` or per-user default), owner-only file creation, GORM models, schema migration, feature catalog seeding, scan job/result storage and queries. |
| Placeholders | `internal/logic-tui.go`, `internal/ui-form.go` | Empty files (package clause only), scheduled for removal because Salus is CLI-only (Q-001, `plan.md` P5-2). |

All application code lives in the single package
`github.com/jabbott-iii/Salus/internal`. Dependency direction today is:
CLI → checks, reporting, persistence. Check and report code does not call
persistence functions or touch the database, and must stay that way. The
only shared symbol is the `DatabasePathEnv` constant, which the `misconfig`
check reads. `DatabasePathEnv` and `DefaultDatabasePath` are defined once in
`internal/database.go` and used by `main`. See `map.md` for diagrams.

### Runtime flow

1. `main` opens (or creates) the SQLite file and runs `AutoMigrate`.
2. `EnsureDefaultFeatures` idempotently seeds `FeatureCategory` and `Feature`
   rows from the compiled-in catalog in `seed.go`.
3. `check run` calls `RunChecks`, which executes checks sequentially in
   `AllCheckKeys` order (or the `--only` order).
4. Unless `--no-save` is set, `RecordScan` persists one `ScanJob` and one
   `ScanResult` per outcome in a single transaction.
5. Output is written as text or JSON (nothing with `--quiet`). A WARN or
   FAIL result is returned as `*ExitStatusError`. `main.run` closes the
   database and returns the exit code (0/1/2, or 3 for operational errors).

## 3. Public contracts (treat as compatibility surface)

Changing any of the following is a breaking change and requires explicit
authorization plus README and `history.md` updates:

- **Command names and flags** documented in `README.md`, including the root
  `--version` flag (output `salus version <version>`, where `<version>` is
  `dev` unless set with `-ldflags "-X main.version=..."`).
- **Exit codes:** `check run` exits `0` all PASS, `1` any WARN, `2` any FAIL
  (`ExitCodeFor`). Every command exits `3` (`ExitCodeError`) for operational
  errors: Cobra flag/argument errors, unknown `--only` keys, missing jobs, and
  database failures (Q-004). `check run` returns `*ExitStatusError` for WARN
  and FAIL, and `main.run` maps any returned error with `ExitCode`. Only
  `main` calls `os.Exit`.
- **Check keys:** `disk-space`, `memory`, `cpu-load`, `docker-status`,
  `kubernetes-status`, `service-uptime`, `misconfig`. Keys are stored in the
  database and accepted by `--only`; never rename a key without a migration.
- **JSON output shape:** an array of objects with `key`, `status`, `message`,
  and `duration_ns` (nanoseconds, from `time.Duration`).
- **`--quiet`** suppresses `check run` report output, including `--json`, and
  still sets the exit code.
- **Output streams:** stdout carries only command output (reports, JSON, help,
  version). Errors go to stderr as `Error: <message>` followed by
  `Run '<command> --help' for usage.`. The root command sets `SilenceErrors`
  and `SilenceUsage`, and `main.run` prints the error. Cobra must never print
  errors or usage itself, because it routes them through the output writer.
- **Unknown input is an error:** group commands (`check`, `jobs`) use
  `runGroup`, which rejects unknown subcommands (with suggestions) instead of
  printing help and exiting 0. Leaf commands declare `Args` (`cobra.NoArgs`,
  `cobra.ExactArgs(1)`).
- **Environment variable:** `SALUS_DB_PATH` overrides the database path. The
  default is per-user (Q-002, since v1.0.1; v1.0.0 used
  `./salus.db`): `$XDG_DATA_HOME/salus/salus.db` or
  `~/.local/share/salus/salus.db` on Linux and other Unix,
  `~/Library/Application Support/salus/salus.db` on macOS, and
  `%LOCALAPPDATA%\salus\salus.db` on Windows (`DefaultDatabasePath`).
  Changing these paths is a breaking change for existing users' history.
- **`--service` values** must be systemd unit names (`validUnitName`).
  Anything else fails the check without running `systemctl` (SEC-001).
- **Database schema:** tables for `FeatureCategory`, `Feature`, `ScanJob`,
  `ScanResult` managed by GORM `AutoMigrate`.

## 4. Conventions

### Source files
- Every Go source file carries the Apache-2.0 license header used throughout
  the repository. Build-constraint lines (`//go:build ...`) go above it.
- Format with `gofmt -s -w .`. CI runs `go vet` and `golangci-lint` (v2.13.2,
  default linter set; there is no `.golangci.yml`).
- Existing file names use hyphens (`logic-cli.go`, `scan-store.go`). Keep the
  existing names; do not rename files without an explicit request.

### Errors and output
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Use `ErrNotFound` (wrapped) for missing records; callers test with
  `errors.Is`.
- Every write to a command's output writer must check its error and return it.
  This was established by PR #12 to satisfy `errcheck`; tests in
  `logic-cli_test.go` enforce it with a failing writer.
- Commands write through `cmd.OutOrStdout()`, never directly to `os.Stdout`,
  so they remain testable.

### Checks
- A check is a `checkFunc` (`func(CheckOptions) CheckOutcome`) and must never
  panic or return an error; failures are expressed as `StatusWarn` or
  `StatusFail` with a human-readable `Message`.
- Use `StatusWarn` when a check cannot run on this host (tool missing,
  unsupported OS) and `StatusFail` when the thing being checked is unhealthy
  or unreachable.
- External tools are run only through `opts.hasTool` and `opts.command`,
  which wrap `exec.LookPath` and `exec.CommandContext` with
  `opts.commandTimeout()` (default 3s). Arguments are passed separately with
  no shell. Tests replace them through the unexported `lookPath` and
  `runCommand` fields of `CheckOptions` (see `fakeToolOptions` in
  `internal/checks_test.go`). Validate any user-supplied argument before
  passing it, and end option parsing with `--` before it where the tool
  supports it (see `SEC-001` in `cybersec.md`; `--service` is validated by
  `validUnitName` and passed as `systemctl is-active -- <name>`).
- `check run` validates `--only` keys (`ValidateCheckKeys`) and opens the
  database before running any check, so input and storage errors are
  reported before slow checks run.
- Outcome messages are a single line. Use `firstLine` on tool output.
- Platform-specific logic uses `_linux.go` / `_other.go` files with matching
  build constraints, and every platform must define every function the
  registry references.
- Threshold defaults and their accessors live in `health-thresholds.go`.
  Zero or negative option values fall back to the defaults via `orDefault`.
  The file is constrained to `//go:build linux` because only the Linux
  resource checks use it. Widen the constraint when macOS and Windows checks
  are added (P3-7).
- Unexported code referenced only from platform-specific files must carry
  the same build constraint. Otherwise golangci-lint's `unused` check fails
  on the other operating systems (this broke the macOS CI job on
  2026-09-27). Lint for all three targets (see section 6).

### Adding a new check (checklist)
1. Add a `key...` constant and append it to `AllCheckKeys` in `health.go`.
2. Implement the check and register it in `checkRegistry`.
3. Add a `defaultFeature` entry (and category, if new) in `seed.go`. Without
   this, `RecordScan` fails with `ErrNotFound` for the new key.
4. Add unit tests that do not depend on ambient host state (use fixtures or
   an injectable runner; see `plan.md`).
5. Update `README.md` features, `map.md`, and `history.md` as applicable.

### Persistence
- Schema changes go through the GORM model structs and `AutoMigrate`.
  `AutoMigrate` only adds; it does not drop or rename columns. Any destructive
  change needs an explicit migration plan recorded in `plan.md` first.
- Multi-row writes belong in a single `Transaction`. Queries inside a
  transaction must use the transaction handle (`tx`), not `db.Conn()`.
- `RecordScan` takes the time the checks started, so `ScanJob.StartedAt` and
  `FinishedAt` bracket the actual run.
- GORM's logger is set to `logger.Silent` in `NewDatabase`. The default logger
  writes to stdout (corrupting `--json`) and logs normal "record not found"
  lookups. Database errors are returned and reported by the CLI.
- Commands receive a `DatabaseOpener` and call it only when they need
  storage. `--help`, `--version`, `completion`, and `check run --no-save`
  never create a database. `main.run` opens the database at most once.
- `NewDatabase` creates a missing database file with mode `0600` and missing
  parent directories with `0700` before SQLite opens it (SEC-004). It never
  touches an existing file, which may be read-only. The file is derived with
  `databaseFile`, which mirrors go-sqlite3's handling of `?` parameters and
  skips `:memory:` and `file:` URIs; the `misconfig` check uses the same
  helper.
- Stored result messages are capped at 1024 bytes (`truncateMessage`).
- Every `NewDatabase` must be paired with `Close`: `main.run` defers it, and
  `newTestDatabase` registers it with `t.Cleanup`. Windows cannot delete an
  open SQLite file, and `t.TempDir` cleanup fails if the handle stays open
  (this failed the Windows CI job on 2026-09-27).
- Tests use a file-backed database in `t.TempDir()` (`newTestDatabase`,
  `newSeededTestDatabase`); do not use `:memory:`, because each pooled
  connection would see a different database.

## 5. Build, platform, and dependency constraints

- **Go version:** `go 1.26.8` in `go.mod`. CI, CD, and the Security workflow
  install exactly this version through `setup-go`'s `go-version-file`, so it
  decides which standard-library security fixes ship in release binaries
  (SEC-008). Keep it at the latest patch release of the Go minor version in
  use. `govulncheck` fails when it falls behind on a reachable fix.
- The Docker builder (`golang:1.26-alpine3.24`) sets `GOTOOLCHAIN=local`, so
  it must provide at least the `go.mod` version, or the image build fails.
  Bump the `go` directive and the builder image together.
- **CGO is required.** `gorm.io/driver/sqlite` uses `github.com/mattn/go-sqlite3`.
  A `CGO_ENABLED=0` build compiles but cannot open the database at runtime.
  Every build needs a C toolchain.
- **Resource checks are Linux-only today.** Disk, memory, CPU load, and host
  uptime read `/proc` and `statfs`. On other platforms they return `WARN`, so
  `check run` exits `1` on macOS and Windows. macOS and Windows
  implementations are approved (Q-005, `plan.md` P3-7). Prefer the standard
  library `syscall` package. Adding `golang.org/x/sys` requires a recorded
  reason and a `NOTICE` update.
- **Direct dependencies:** `spf13/cobra`, `gorm.io/gorm`,
  `gorm.io/driver/sqlite`. Do not add dependencies without a documented
  reason in `plan.md` or `notes.md`; prefer the standard library.
- Keep `NOTICE` in sync with the actual module graph whenever dependencies
  change.

## 6. Testing expectations

- `go test ./...` must pass on Linux, macOS, and Windows (CI matrix).
- `golangci-lint run ./...` must pass for every CI target. From Linux, check
  the other targets with `GOOS=darwin golangci-lint run ./...` and
  `GOOS=windows golangci-lint run ./...` (CGO is off by default when cross
  targeting, which is enough for linting).
- Tests must be deterministic and must not execute Docker, Kubernetes, or
  systemd tools, or depend on host resource levels. Use `fakeToolOptions` for
  external tools, and fixture data with the `parse*` functions for `/proc`
  contents (`internal/health-resources_linux_test.go`).
- Tests that read environment-dependent checks (`misconfig`) call
  `isolateMisconfigEnv`. OS-specific expectations use `runtime.GOOS` with
  `t.Skip`, never silent passes.
- Commands are tested through `Execute()` with injected writers and
  arguments. The whole CLI, including exit codes, is tested through
  `main.run`.

## 7. CI/CD expectations

- `ci.yml`: `go mod tidy` drift check, `go vet`, `golangci-lint`, tests with
  coverage (Codecov), and a native build plus smoke test on each OS.
- `security.yml`: CodeQL (`security-extended`) and gosec (SARIF upload).
  gosec is intentionally non-blocking (Q-009). Its findings must be triaged
  in GitHub Code Scanning rather than ignored.
- `docker.yml`: image build plus smoke tests on `main` and PRs to `main`.
- `cd.yml`: on `v*` tags, builds six OS/arch targets with CGO, injects the
  tag with `-X main.version`, smoke tests, packages, generates checksums, and
  publishes a GitHub Release. The canonical release format is
  `salus_<os>_<arch>.tar.gz` (Linux, macOS) and `salus_<os>_<arch>.zip`
  (Windows), plus `checksums.txt` (Q-003). The README install section must
  match it.
- Third-party actions are pinned by commit SHA with a version comment. Keep
  that practice for every new action. Dependabot (`.github/dependabot.yml`:
  `gomod`, `github-actions`, `docker`, weekly) updates the pins. Tools run with
  `go run tool@version` (for example `govulncheck` in `security.yml`) are not
  seen by Dependabot and must be bumped by hand.
- Some actions must always move together in one change:
  - Every `github/codeql-action` sub-action (`init`, `autobuild`, `analyze`,
    `upload-sarif`). A split bump fails CodeQL with a configuration-version
    mismatch; this was seen on Dependabot PR #17.
  - `actions/upload-artifact` and `actions/download-artifact`.

  Dependabot groups them once P2-8 (a) is applied. The artifact actions run
  only in `cd.yml`, so PR checks do not cover them. Test them with a
  `workflow_dispatch` CD run; its `Create GitHub Release` step runs only for
  tags.
- `security.yml` also runs `govulncheck`, which fails on vulnerabilities
  reachable from Salus code (SEC-005). Unlike gosec, it is blocking.
- Container image (`Dockerfile`):
  - Both stages are pinned by digest on the same Alpine release, so the binary
    runs against the musl it was linked with. Bump the builder and runtime
    tags together. Dependabot only automates digest and patch updates, grouped
    into one PR.
  - The runtime stage installs no packages: go-sqlite3 compiles SQLite into
    the binary, and Salus makes no TLS connections. Add packages only with a
    concrete need.
  - The runtime runs as UID/GID 10001 (`salus`), which owns `/app/data`; the
    directory must be created and chowned before `VOLUME`.
  - `.dockerignore` keeps local state and secrets out of the build context
    (SEC-003).
  - The image does not ship `docker` or `kubectl`, so those checks report WARN
    in-container by design (SEC-007).
  - `docker.yml` asserts the non-root user, volume persistence, and those
    WARN results.
- Smoke tests and artifact names must use the Salus binary name, the
  `SALUS_DB_PATH` variable, and real Salus commands. Smoke steps accept
  `check run` exit codes 0 and 1 only, because runner host state varies, and
  assert persistence with `jobs show`. (`7235211` used another project's
  names and commands; this was corrected in Phase 0 on 2026-09-27.)
- CI triggers on `push` to every branch and `pull_request` to every branch.
  Keep both (Q-008).
- Releases are cut with `make release VERSION=vX.Y.Z`, which creates and
  pushes an annotated tag.

## 8. Documentation duties

See `AGENTS.md` for the full rules. In short: `map.md` changes with structure,
`cybersec.md` changes with security findings, `history.md` is append-only,
`plan.md` holds active work, and `notes.md` holds durable notes and open
questions.
