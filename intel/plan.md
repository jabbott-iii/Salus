# Implementation Plan

Active implementation plans and follow-on work. Architecture rules are in
[`maint.md`](maint.md). Security items (`SEC-*`) are defined in
[`cybersec.md`](cybersec.md), and open questions (`Q-*`) in [`notes.md`](notes.md).

Last reviewed: 2026-09-27 (against commit `7235211`).

Status values: `Proposed` (not started), `Ready` (decision made, can start),
`In Progress`, `Blocked`, `Done`.

## Baseline (verified 2026-09-27)

Go sources at local `HEAD` (`7235211`) are byte-identical to `origin/main`
(`31106f9`). The commit ahead changes only workflows, `CONTRIBUTING.md`,
`NOTICE`, and `intel/golang.md`. Commands were run on Linux/amd64 with Go
1.26.8 built from source (see "Verification environment" in `notes.md`).

| Check | Result |
|---|---|
| `gofmt -s -l .` | No files listed |
| `go vet ./...` | Pass |
| `go test ./...` | Pass (root package 27.3%, `internal` 66.0%, total 64.7% of statements) |
| `go test -race ./...` | Pass |
| `go build` with `-ldflags "-X main.version=..."` | Builds. The `-X` flag is silently ignored because `main.version` does not exist |
| `salus --version` (CI/CD smoke step) | **Fails:** `unknown flag: --version`, exit 1 |
| `salus add --title ...` (CI/CD/Docker smoke step) | **Fails:** `unknown command "add"`, exit 1 |
| `MUNUS_DB_PATH=...` (CI/CD smoke env) | Ignored. `salus.db` is created in the current directory |
| `salus check list`, `check run --only misconfig`, `jobs list`, `jobs show 1` | Work as documented, exit 0 |
| `check run --only nope` | Exit 1, the same code as WARN |
| `check run --quiet --json` | Prints JSON |
| New database file mode | `644` |
| `--service=--host=user@example.invalid` (fake `systemctl` on PATH) | `systemctl` received `is-active --host=user@example.invalid`, confirming SEC-001 |
| Coverage gaps | `NewRootCmd`, `newCheckCmd`, `newCheckRunCmd`, `newJobsCmd`, `main` at 0% |

Not run: `golangci-lint`, gosec, CodeQL, `govulncheck`, the Docker image build,
and macOS/Windows tests (tooling or network unavailable in the analysis
environment). GitHub Actions results were not inspected.

## Phase 0: Make the pipeline match Salus (blocker before pushing `7235211`)

Goal: CI, Docker, and CD workflows build, smoke-test, and release Salus.

| ID | Work | Acceptance criteria | Status |
|---|---|---|---|
| P0-1 | Add version reporting: `var version = "dev"` in `main.go`, passed to the root command's `Version` field so `-X main.version=` takes effect. | `salus --version` prints the injected version. A unit test covers the default. | Proposed |
| P0-2 | Fix the `ci.yml` smoke step: binary `salus-ci`, `SALUS_DB_PATH`, commands `--version`, `check list`, `check run --only misconfig` (accept exit 0 or 1, fail on 2 or higher; see P1-5 for Windows), then `jobs show 1 \| grep -q misconfig`. | The CI matrix is green on ubuntu, macOS, and Windows. | Proposed |
| P0-3 | Fix `cd.yml`: `munus` → `salus` in artifact names, comments, env var, and smoke commands (same as P0-2). Resolve Q-003 and make packaging match the README. | A tag on a fork or test branch produces six `salus_*` artifacts plus `checksums.txt`. Every smoke-enabled target passes. | Blocked on Q-003 |
| P0-4 | Fix `docker.yml`: image tag `salus:<sha>`, volume `salus-smoke`, commands `check run --only misconfig` and `jobs list`. Either implement the non-root user (SEC-002) or remove the non-root comment. | The Docker workflow is green on a PR to `main`. | Proposed |
| P0-5 | Restore `NOTICE` third-party entries to match `go.mod`: cobra (Apache-2.0), pflag (BSD-3-Clause), mousetrap (Apache-2.0), gorm (MIT), gorm sqlite driver (MIT), go-sqlite3 (MIT, bundles public-domain SQLite), inflection (MIT), now (MIT), x/text (BSD-3-Clause). | `NOTICE` lists every module in `go.mod` and nothing else. | Blocked on Q-006 |

## Phase 1: Correctness and testability

| ID | Work | Acceptance criteria | Status |
|---|---|---|---|
| P1-1 | Remove `os.Exit` from `check run`'s `RunE`. Return a typed exit-status error and map it to the exit code in `main`, keeping codes 0/1/2 unchanged. | Table tests execute `check run` through `cmd.Execute()` and cover exit status, `--json`, `--fail-only`, `--quiet`, and `--no-save`. `newCheckRunCmd` coverage is above 80%. | Proposed |
| P1-2 | `RecordScan`: use `tx` for `featureByKey`, and record the real start time (captured before `RunChecks`) so `StartedAt` and `FinishedAt` reflect execution. | A test asserts `StartedAt <= FinishedAt` with a measurable gap for a slow fake check. Feature lookups use `tx`. | Proposed |
| P1-3 | Define `SALUS_DB_PATH` and the default path once, and consume them from both `main` and `internal`. | One definition. Existing tests still pass. | Proposed |
| P1-4 | Close the database (the underlying `*sql.DB`) before exit. | A clean shutdown path exists. No behavior change. | Proposed |
| P1-5 | `misconfig`: skip the POSIX permission-bit check on Windows. Go reports `0666` for any non-read-only file there (`os/types_windows.go`), which causes a false WARN whenever `SALUS_DB_PATH` is set. Inferred from the Go source, not yet observed on Windows. | A Windows CI test with `SALUS_DB_PATH` set shows `misconfig` returning PASS. | Proposed |
| P1-6 | Define `--quiet` with `--json` behavior: make them mutually exclusive (Cobra `MarkFlagsMutuallyExclusive`), or let quiet win. | The documented behavior is covered by a test and the README is updated. | Proposed |
| P1-7 | Keep outcome messages single-line. `service-uptime` currently embeds the full combined `systemctl` output. | A test with multi-line fake output yields a single-line message. | Proposed |
| P1-8 | Make checks testable without host state: inject a command runner (`exec` wrapper) and file readers. Add fixture-based parser tests for `/proc/meminfo`, `/proc/loadavg`, and `/proc/uptime`, and fake-runner tests for docker, kubectl, and systemctl paths. Stop `TestRunChecksDefaultsToAllChecks` from executing real CLIs. | No test shells out to real `docker`, `kubectl`, or `systemctl`. `internal` coverage is above 80%. | Proposed |
| P1-9 | Use a distinct exit code for operational errors (Q-004). | The decision is recorded, and implemented with README and `history.md` updates if approved. | Blocked on Q-004 |
| P1-10 | Open the database only for commands that need it, not for `--help`, `completion`, or `check run --no-save`. Related to Q-002. | Running `salus --help` in an empty directory creates no file. | Blocked on Q-002 |

## Phase 2: Security hardening

Order by risk and effort. Update `cybersec.md` status as each item moves.

| ID | Work | Depends on | Status |
|---|---|---|---|
| P2-1 | SEC-001: validate `--service` and pass `--` to `systemctl`. | P1-8 (runner injection makes the test clean) | Proposed |
| P2-2 | SEC-004: create the DB file with `0600`, check the effective DB path in `misconfig` (including read bits), and bound message length. | P1-3, P1-5 | Proposed |
| P2-3 | SEC-002 and SEC-003: non-root container user, digest-pinned base images, `.dockerignore`. | P0-4 | Proposed |
| P2-4 | SEC-005: `govulncheck` in CI, `.github/dependabot.yml` (gomod, github-actions, docker), gosec gating policy (Q-009). | none | Proposed |
| P2-5 | SEC-007: README container guidance. | P4-1 | Proposed |
| P2-6 | SEC-006: release provenance or signing. | P0-3 | Proposed |

## Phase 3: Feature completeness

Scope comes from the project description (system health, Docker, Kubernetes,
memory, CPU, disk, service uptime, misconfiguration detection). Each item
needs tests without host dependence (P1-8) and README updates.

| ID | Work | Notes | Status |
|---|---|---|---|
| P3-1 | Expose thresholds and the command timeout as flags on `check run` (for example `--disk-warn`, `--disk-fail`, `--mem-warn`, `--mem-fail`, `--load-warn`, `--load-fail`, `--timeout`). Validate warn < fail and sane ranges. | `CheckOptions` already supports these fields. This is additive and non-breaking. | Proposed |
| P3-2 | Docker container health: report unhealthy or restarting containers, not only daemon reachability. | Matches the "Container Runtime" category description in `seed.go`. | Proposed |
| P3-3 | Kubernetes depth: node readiness, plus an optional `--kube-context`. | Keep `kubectl` as the integration. No client-go dependency without a decision. | Proposed |
| P3-4 | Broader misconfiguration detection: kubeconfig permissions, Docker socket permissions, world-writable `PATH` entries, and DB file mode (with P2-2). | Every rule gets a stable identifier in the message and a test. | Proposed |
| P3-5 | Machine-readable history: `jobs list --json` and `jobs show --json`. | Additive. Does not change the `check run --json` array shape. | Proposed |
| P3-6 | Job retention: a way to prune old jobs (for example `jobs prune --older-than 30d`). | Prevents unbounded DB growth under cron. | Proposed |
| P3-7 | macOS and Windows resource checks (Q-005). | Likely needs `golang.org/x/sys`. Requires a dependency decision. | Blocked on Q-005 |
| P3-8 | Multiple services in one run (for example a repeatable `--service`). | Output and storage use one outcome per key today. Needs a design for per-service keys. | Proposed |

## Phase 4: Documentation and developer experience

| ID | Work | Status |
|---|---|---|
| P4-1 | Align the README with the `AGENTS.md` section order: prerequisites (Go 1.26, C toolchain for CGO), build from source, configuration (`SALUS_DB_PATH`), exit codes, testing and quality checks, project structure, accurate install steps (Q-003), and container caveats (SEC-007). | Proposed |
| P4-2 | Add Makefile targets `build`, `test`, `vet`, `lint`, `fmt`, and `cover`, keeping the existing release targets unchanged. | Proposed |
| P4-3 | Add an explicit `.golangci.yml` so the linter set does not drift with golangci-lint defaults. | Proposed |
| P4-4 | Add `SECURITY.md` with a vulnerability reporting channel. Needs maintainer input on the channel. | Proposed |
| P4-5 | Resolve `.idea/` handling (Q-007). Add issue and PR templates that match `CONTRIBUTING.md`'s issue-first rule. | Proposed |
| P4-6 | Dockerfile cleanup: fix the stale "rete.db" and "TUI" comments, and use `TARGETARCH` instead of a hard-coded `GOARCH=amd64`. Check whether `sqlite-libs` is needed at runtime (go-sqlite3 bundles SQLite unless built with the `libsqlite3` tag) using `ldd` before removing it. | Proposed |

## Phase 5: Structure (optional, needs explicit approval)

| ID | Work | Status |
|---|---|---|
| P5-1 | Once the package grows, split `internal` into focused packages (for example `internal/checks`, `internal/store`, `internal/report`, `internal/cli`). Rename hyphenated files only as part of that move. | Proposed |
| P5-2 | Implement or remove the TUI placeholders (Q-001). | Blocked on Q-001 |

## Recommended sequence

1. **M1, green pipeline:** P0-1, P0-2, P0-4, then P0-3 and P0-5 once Q-003
   and Q-006 are answered. Push `7235211` only together with these fixes.
2. **M2, testable core:** P1-1, P1-8, P1-2, P1-5, P1-3, P1-4, P1-7, P1-6.
3. **M3, hardened:** P2-1 through P2-4, and P2-5 with P4-1.
4. **M4, first tagged release (`v0.1.0`):** P4-1, P4-2, P2-6, P3-1.
5. **M5, depth:** P3-2 through P3-6, then P3-7 and P3-8 per decisions.
