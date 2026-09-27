# Repository History

Append-only record of significant repository changes. Do not edit, reorder,
or delete existing entries. Add new entries at the end.

Entry format:

```text
## YYYY-MM-DD: Short title
- Change: what changed
- Files: key files or directories
- Reason / reference: why, plus commit or PR where known
```

## 2026-07-25 to 2026-09-21: Reconstructed history (from `git log`, recorded 2026-09-27)

The entries below summarize history before this log existed. They are
reconstructed from commit messages and diffs, not first-hand records.

- 2026-07-25 to 2026-07-27: Repository initialized. Apache-2.0 `LICENSE` and a
  `NOTICE` file added (`9f9999c`, `a08ccaa`, `d0a1c4f`).
- 2026-07-26 to 2026-08-27: Initial application structure, README,
  devcontainer, and workflows iterated under earlier project names. Commit
  messages mention "Capsus" (`14d7c7c`) and "Rete" (`3a67f29`, `8d90da9`).
  CSV-based feature seeding replaced by a compiled-in default catalog
  (PR #6, `37af05d`).
- 2026-09-03 to 2026-09-04: Transition from Rete to Salus. Devcontainer
  renamed (`3a67f29`), old code removed (`8d90da9`), and the environment
  health checker CLI implemented with Cobra and GORM/SQLite (`b53900f`,
  merged in `3b752f2`).
- 2026-09-06: CI expanded to a multi-OS matrix with coverage. Security
  workflow gained gosec. Go version set to 1.26.0. Code formatted with
  `gofmt -s` (`72f7fcd`, `3be62db`, `d86e177`, `51e57a6`).
- 2026-09-17: Dependency update (`19fef45`). CLI and report writers now
  propagate write errors to satisfy `errcheck`, with CLI tests added
  (PR #12, `63d0658`, `1146cdc`).
- 2026-09-21: `AGENTS.md` added (`31106f9`).

## 2026-09-27: CI/CD workflows updated (local commit, not yet pushed)

- Change: Third-party actions pinned by commit SHA. CD rewritten for native
  CGO builds on six OS/arch targets with packaging, checksums, and GitHub
  Release. Smoke tests added to CI, CD, and Docker workflows.
  `intel/golang.md` added. `CONTRIBUTING.md` content removed. The `NOTICE`
  third-party list removed.
- Files: `.github/workflows/*.yml`, `intel/golang.md`, `CONTRIBUTING.md`,
  `NOTICE`
- Reason / reference: Commit `7235211`. Analysis the same day found that the
  smoke tests and artifact names target a different binary ("munus"), so
  they will fail for Salus. See `notes.md` and `plan.md` Phase 0.

## 2026-09-27: Repository intelligence documents created; CONTRIBUTING.md restored

- Change: Full repository analysis performed. Created the documents
  `AGENTS.md` requires: `intel/maint.md`, `intel/map.md`,
  `intel/cybersec.md` (SEC-001 to SEC-007, all Open), `intel/notes.md`
  (engineering notes, open questions Q-001 to Q-009), `intel/plan.md`
  (verified baseline and phased plan), and this file. Populated the empty
  `CONTRIBUTING.md`, restoring the contribution rules removed in `7235211`
  and adding setup, validation, and pull-request expectations consistent with
  `intel/maint.md`.
- Files: `intel/maint.md`, `intel/map.md`, `intel/cybersec.md`,
  `intel/notes.md`, `intel/plan.md`, `intel/history.md`, `CONTRIBUTING.md`
- Reason / reference: The documents did not exist, and `AGENTS.md` requires
  them. No source code, workflows, or configuration were changed.
  Uncommitted at the time of writing.

## 2026-09-27: Maintainer decisions recorded; Phase 0 pipeline fixes

- Change: Recorded decisions on Q-001 to Q-009: CLI only, per-user data
  directory, archive releases, distinct operational exit code, macOS and
  Windows resource checks, intentional `CONTRIBUTING.md`/`NOTICE` reset,
  `.idea/` tracked, CI triggers unchanged, gosec non-blocking. Implemented
  Phase 0:
  - Added a root `--version` flag backed by `main.version`, which makes the
    `-X main.version` release ldflag effective.
  - In the CI, CD, and Docker workflows, changed the binary, artifact,
    image, and volume names from "munus" to "salus", and the database
    variable to `SALUS_DB_PATH`.
  - Replaced the invalid smoke commands with `--version`, `check list`,
    `check run --only misconfig` (exit codes 0 and 1 accepted), and
    `jobs show 1`.
  - Filled `NOTICE` with the modules actually linked into the binary.
  - Rewrote the README install section for `.tar.gz`/`.zip` archives with
    checksum verification.
- Files: `version.go`, `version_test.go`, `main.go`,
  `.github/workflows/ci.yml`, `.github/workflows/cd.yml`,
  `.github/workflows/docker.yml`, `NOTICE`, `README.md`, `intel/notes.md`,
  `intel/plan.md`, `intel/cybersec.md`, `intel/maint.md`, `intel/map.md`
- Reason / reference: `7235211` (pushed with `460a24b`) referenced another
  project's binary and commands, so its smoke steps cannot pass. Validation
  is in `plan.md` ("Phase 0 validation"). GitHub Actions runs are still
  pending. The workflow edits were delivered as a patch
  (`salus-phase0-workflows.patch`, applied with `git apply`), because the
  remote session cannot write `.github/workflows/`. Uncommitted at the time
  of writing.

## 2026-09-27: macOS/Windows lint failure fixed (threshold helpers build-constrained)

- Change: Moved the resource-threshold defaults, `orDefault`, and the six
  `CheckOptions` threshold accessors from `internal/health.go` into the new
  `internal/health-thresholds.go`, constrained to `//go:build linux`. No
  behavior change on Linux. Added cross-OS lint commands to
  `CONTRIBUTING.md` and the matching rule to `intel/maint.md`.
- Files: `internal/health.go`, `internal/health-thresholds.go`,
  `CONTRIBUTING.md`, `intel/maint.md`, `intel/map.md`, `intel/plan.md`
- Reason / reference: CI run 36305462889 on `586dfe9` failed on macOS at
  `golangci-lint` (7 `unused` findings), and Windows was cancelled by
  fail-fast. Those helpers were used only by the Linux resource checks, so
  they were dead code on other targets. The affected code and the
  golangci-lint version (v2.13.2) both predate Phase 0. Earlier CI runs were
  not inspected. Uncommitted at the time of writing.

## 2026-09-27: M2 (testable core): exit codes, DB close, test seams, Windows CI fix

- Change:
  - `check run` no longer calls `os.Exit`. It returns `*ExitStatusError` for
    WARN/FAIL, and `main.run` maps the result to the exit code: 0/1/2, and
    the new `3` for operational errors (invalid flags or arguments, unknown
    check, missing job, database failure).
  - Added `Database.Close`. `main.run` defers it, and test databases close in
    `t.Cleanup`.
  - `RecordScan` now takes the checks' start time and looks up features
    inside the transaction.
  - `SALUS_DB_PATH` and the default path are defined once
    (`internal.DatabasePathEnv`, `internal.DefaultDatabasePath`).
  - `misconfig` skips the POSIX permission test on Windows.
  - `--quiet` suppresses all output, including `--json`.
  - `service-uptime` messages are single-line.
  - Errors go only to stderr (`Error: …` plus a `--help` hint), and Cobra's
    own error and usage printing is silenced.
  - `check` and `jobs` reject unknown subcommands (previously help and exit
    0). Leaf commands reject extra arguments.
  - GORM's logger is silenced (it wrote "record not found" lines to stdout).
  - Added test seams for external tools (`lookPath`/`runCommand` on
    `CheckOptions`) and `/proc` parse functions, with tests: fake-tool check
    tests, parser fixtures, `check run` and group-command tests, and
    end-to-end exit codes through `main.run`. `internal` coverage went from
    66% to 87.7%.
- Files: `main.go`, `main_test.go`, `database_path.go`,
  `database_path_test.go`, `internal/database.go`,
  `internal/database_test.go`, `internal/health.go`,
  `internal/health-resources_linux.go`, `internal/health_test.go`,
  `internal/checks_test.go`, `internal/health-resources_linux_test.go`,
  `internal/logic-cli.go`, `internal/logic-cli_test.go`, `internal/report.go`,
  `internal/scan-store.go`, `internal/scan-store_test.go`, `README.md`,
  `intel/maint.md`, `intel/map.md`, `intel/notes.md`, `intel/plan.md`,
  `intel/cybersec.md`
- Reason / reference: Plan milestone M2 (P1-1 to P1-9, plus P1-11). An
  independent review caught a regression before commit (usage text on stdout
  once `run` set the output writer), fixed under P1-11. CI run 36343455341 on
  `4995446` failed on Windows because the tests never closed the SQLite file,
  so `t.TempDir` cleanup could not delete it. Exit code `3` is a
  maintainer-approved change to the public exit-code contract (Q-004).
  Uncommitted at the time of writing.
