# Architecture and Maintainability Guidance

This document is the authoritative source for Salus architecture and
maintainability rules. `CONTRIBUTING.md` summarizes the contributor-facing
subset; if the two disagree, this document wins and `CONTRIBUTING.md` must be
corrected. Go language rules live in [`golang.md`](golang.md), which
`AGENTS.md` designates as the authoritative guidance on Go language usage. They
apply to all Go work in this repository.

Last reviewed: 2026-09-27 (against commit `460a24b` plus uncommitted Phase 0
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
| Entry point | `main.go`, `database_path.go`, `version.go` | Resolve DB path from `SALUS_DB_PATH`, open DB, seed catalog, build the root command with the build `version` (`--version`), and execute it. |
| CLI | `internal/logic-cli.go` | Cobra command tree (`check list`, `check run`, `jobs list`, `jobs show`), flag parsing, exit-code mapping. |
| Checks | `internal/health.go`, `internal/health-resources_linux.go`, `internal/health-resources_other.go` | Check registry, thresholds, and the individual check functions. |
| Reporting | `internal/report.go` | Text and JSON rendering, worst-status aggregation, exit-code mapping. |
| Persistence | `internal/database.go`, `internal/scan-store.go`, `internal/seed.go` | GORM models, schema migration, feature catalog seeding, scan job/result storage and queries. |
| Placeholders | `internal/logic-tui.go`, `internal/ui-form.go` | Empty files (package clause only), scheduled for removal because Salus is CLI-only (Q-001, `plan.md` P5-2). |

All application code lives in the single package
`github.com/jabbott-iii/Salus/internal`. Dependency direction today is:
CLI → checks, reporting, persistence. Check and report code does not call
persistence functions or touch the database, and must stay that way. The
only shared symbol is the `databasePathEnvVar` constant, which the
`misconfig` check reads. See `map.md` for diagrams.

### Runtime flow

1. `main` opens (or creates) the SQLite file and runs `AutoMigrate`.
2. `EnsureDefaultFeatures` idempotently seeds `FeatureCategory` and `Feature`
   rows from the compiled-in catalog in `seed.go`.
3. `check run` calls `RunChecks`, which executes checks sequentially in
   `AllCheckKeys` order (or the `--only` order).
4. Unless `--no-save` is set, `RecordScan` persists one `ScanJob` and one
   `ScanResult` per outcome in a single transaction.
5. Output is written as text or JSON, then the process exits with the code
   derived from the worst status.

## 3. Public contracts (treat as compatibility surface)

Changing any of the following is a breaking change and requires explicit
authorization plus README and `history.md` updates:

- **Command names and flags** documented in `README.md`, including the root
  `--version` flag (output `salus version <version>`, where `<version>` is
  `dev` unless set with `-ldflags "-X main.version=..."`).
- **Exit codes of `check run`:** `0` all PASS, `1` any WARN, `2` any FAIL
  (`ExitCodeFor`). Cobra/command errors currently also exit `1`. A distinct
  exit code for operational errors is approved (Q-004) and planned as `3`
  (`plan.md` P1-9). Until it lands, treat `1` as ambiguous.
- **Check keys:** `disk-space`, `memory`, `cpu-load`, `docker-status`,
  `kubernetes-status`, `service-uptime`, `misconfig`. Keys are stored in the
  database and accepted by `--only`; never rename a key without a migration.
- **JSON output shape:** an array of objects with `key`, `status`, `message`,
  and `duration_ns` (nanoseconds, from `time.Duration`).
- **Environment variable:** `SALUS_DB_PATH` (default `salus.db` in the current
  working directory). The default is approved to move to a per-user data
  directory (Q-002, `plan.md` P1-10). `SALUS_DB_PATH` will keep overriding it.
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
- External commands go through `exec.CommandContext` with
  `opts.commandTimeout()` (default 3s), with arguments passed separately and
  no shell. Validate any user-supplied argument before passing it (see
  `SEC-001` in `cybersec.md`).
- Platform-specific logic uses `_linux.go` / `_other.go` files with matching
  build constraints, and every platform must define every function the
  registry references.
- Threshold defaults are constants in `health.go`; zero or negative option
  values fall back to the defaults via `orDefault`.

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
  transaction must use the transaction handle (`tx`), not `db.Conn()`
  (current deviation tracked in `plan.md`).
- Tests use a file-backed database in `t.TempDir()` (`newTestDatabase`,
  `newSeededTestDatabase`); do not use `:memory:`, because each pooled
  connection would see a different database.

## 5. Build, platform, and dependency constraints

- **Go version:** `go 1.26.0` in `go.mod`; CI and CD read the version from
  `go.mod`, and the Dockerfile uses `golang:1.26-alpine`. Keep these aligned.
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
- New tests must be deterministic and must not depend on Docker, Kubernetes,
  systemd, or specific host resource levels. Existing
  `TestRunChecksDefaultsToAllChecks` executes real checks and only asserts on
  keys; do not extend that pattern.
- Commands should be testable through `Execute()` with injected writers. The
  `os.Exit` call inside `check run` currently prevents that for `check run`
  (tracked in `plan.md`).

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
  that practice for every new action.
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
