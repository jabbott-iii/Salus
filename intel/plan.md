# Implementation Plan

Active implementation plans and follow-on work. Architecture rules are in
[`maint.md`](maint.md). Security items (`SEC-*`) are defined in
[`cybersec.md`](cybersec.md), and open questions (`Q-*`) in [`notes.md`](notes.md).

Last reviewed: 2026-09-27 (against `753252e` plus uncommitted P2-3/P2-4/P2-5
changes). Decisions on Q-001 to Q-009 are recorded in `notes.md`.

Status values: `Proposed` (not started), `Ready` (decision made, can start),
`In Progress`, `Blocked`, `Awaiting merge` (delivered as a patch for the
maintainer to apply, for example for `.github/`), `Awaiting CI` (implemented
and validated locally, waiting for a GitHub Actions run), `Done`.

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

## Phase 0: Make the pipeline match Salus (CI broken on `main` since `7235211` was pushed)

Goal: CI, Docker, and CD workflows build, smoke-test, and release Salus.

### Phase 0 validation (2026-09-27, uncommitted working tree)

| Check | Result |
|---|---|
| `gofmt -s -l .`, `go vet ./...` | Clean / pass |
| `go test ./...`, `go test -race ./...` | Pass (root package 42.9%, `internal` 65.7%) |
| `go build -ldflags "-s -w -X main.version=v0.0.0-verify"`, then `--version` | Prints `salus version v0.0.0-verify` |
| `actionlint` 1.7.7 with shellcheck 0.11.0 on all workflows | One finding, identical before and after the change: `windows-11-arm` is not in actionlint 1.7.7's runner-label list (pre-existing in `cd.yml`) |
| CI, CD, and Docker smoke scripts extracted from the YAML and run with bash `-eo pipefail` against the built binary (Docker via a local shim) | All pass. A wrong `VERSION` and a `check run` exit code of 2 both fail the step, as intended |
| Release packaging commands from the README (`sha256sum --check`, `shasum -a 256 --check`, `tar -xzf`) against archives built the way `cd.yml` builds them | Pass |

Not run: GitHub Actions itself, the Docker image build, macOS and Windows
execution, and a tag-triggered release.

CI run #44 (`586dfe9`, run 36305462889): ubuntu passed, including the new
smoke step. macOS failed at `golangci-lint` with 7 `unused` findings in
`internal/health.go` (`orDefault` and the six threshold accessors), and
Windows was cancelled by fail-fast. Those helpers are referenced only from
`health-resources_linux.go`, and neither they nor the golangci-lint version
changed in Phase 0 (earlier runs were not inspected). Reproduced locally with
golangci-lint v2.13.2 for `GOOS=darwin` and `GOOS=windows`. Fixed by moving
them into `internal/health-thresholds.go` (`//go:build linux`). After the
move, all three targets report 0 issues. The macOS and Windows smoke steps
have not yet run in CI.

Delivery note: the remote session cannot write `.github/workflows/`, so the
P0-2 to P0-4 edits were handed over as `salus-phase0-workflows.patch`,
checked with `git apply --check` against `460a24b`, with CRLF line endings
preserved in `ci.yml` and `cd.yml`. Until that patch is applied, the
workflow files in the repository still contain the old "munus" steps.

| ID | Work | Acceptance criteria | Status |
|---|---|---|---|
| P0-1 | Add version reporting: `var version = "dev"` in `version.go`, set on the root command's `Version` field so `-X main.version=` takes effect. | `salus --version` prints the injected version. A unit test covers it. | Done |
| P0-2 | Fix the `ci.yml` smoke step: binary `salus-ci`, `SALUS_DB_PATH`, commands `--version`, `check list`, `check run --only misconfig` (accept exit 0 or 1, fail on 2 or higher; see P1-5 for Windows), then `jobs show 1 \| grep misconfig >/dev/null`. (`grep -q` is avoided under `pipefail`: it can exit early and turn the writer's broken pipe into a failure.) | The CI matrix is green on ubuntu, macOS, and Windows. CI #45 (`4995446`): ubuntu and macOS green. Windows failed in tests (unclosed DB, fixed by P1-4). Awaiting the next Windows run. CI #46 (`0d3b91a`, run 36353006078) is green on ubuntu, macOS, and Windows. The smoke step passed on all three. | Done |
| P0-3 | Fix `cd.yml`: `munus` → `salus` in artifact names, comments, env var, and smoke commands (same as P0-2). Q-003 decided: archives are canonical, and the README install section now matches. The smoke step also checks that `--version` output equals `salus version <tag>`. | A tag on a fork or test branch produces six `salus_*` archives plus `checksums.txt`. Every smoke-enabled target passes. Verified by the v1.0.0 release (tag at `231487a`): six `salus_*` archives plus `checksums.txt` published. | Done |
| P0-4 | Fix `docker.yml`: image tag `salus:<sha>`, volume `salus-smoke`, commands `check run --only misconfig` and `jobs show 1`. The inaccurate non-root comment was replaced with a pointer to SEC-002. `--version` added to the smoke step. | The Docker workflow is green on a PR to `main`. The Docker workflow is green on `0d3b91a` (run 36353006162). | Done |
| P0-5 | Restore `NOTICE` third-party entries to match `go.mod`: cobra (Apache-2.0), pflag (BSD-3-Clause), mousetrap (Apache-2.0), gorm (MIT), gorm sqlite driver (MIT), go-sqlite3 (MIT, bundles public-domain SQLite), inflection (MIT), now (MIT), x/text (BSD-3-Clause). | `NOTICE` lists every module linked into the binary, and nothing else. Line endings (CRLF) preserved. | Done |

## Phase 1: Correctness and testability

| ID | Work | Acceptance criteria | Status |
|---|---|---|---|
| P1-1 | Remove `os.Exit` from `check run`'s `RunE`. Return a typed exit-status error and map it to the exit code in `main`, keeping codes 0/1/2 unchanged. | Table tests execute `check run` through `cmd.Execute()` and cover exit status, `--json`, `--fail-only`, `--quiet`, and `--no-save`. `newCheckRunCmd` coverage is above 80%. Done: `check run` returns `*ExitStatusError`, `main.run` maps it. `newCheckRunCmd` coverage 89.7%. | Done |
| P1-2 | `RecordScan`: use `tx` for `featureByKey`, and record the real start time (captured before `RunChecks`) so `StartedAt` and `FinishedAt` reflect execution. | A test asserts `StartedAt <= FinishedAt` with a measurable gap for a slow fake check. Feature lookups use `tx`. Done: `RecordScan(db, startedAt, outcomes)` and `featureByKey(tx, ...)`. `TestRecordScanKeepsStartTime` records a start time 2s in the past and reads the job back from SQLite, instead of using a slow fake check. `TestCheckRunPassRecordsJob` checks `FinishedAt >= StartedAt` end to end. | Done |
| P1-3 | Define `SALUS_DB_PATH` and the default path once, and consume them from both `main` and `internal`. | One definition. Existing tests still pass. Done: `internal.DatabasePathEnv` / `internal.DefaultDatabasePath`. | Done |
| P1-4 | Close the database (the underlying `*sql.DB`) before exit. | A clean shutdown path exists. No behavior change. Done locally: `Database.Close`, deferred in `main.run` and registered with `t.Cleanup` in `newTestDatabase`. Fixes the Windows CI failure (`TempDir RemoveAll cleanup ... being used by another process`, run 36343455341). Awaiting a green Windows run. CI #46 (`0d3b91a`, run 36353006078) is green on ubuntu, macOS, and Windows. The Windows TempDir failure is gone. | Done |
| P1-5 | `misconfig`: skip the POSIX permission-bit check on Windows. Go reports `0666` for any non-read-only file there (`os/types_windows.go`), which causes a false WARN whenever `SALUS_DB_PATH` is set. Inferred from the Go source, not yet observed on Windows. | A Windows CI test with `SALUS_DB_PATH` set shows `misconfig` returning PASS. Done locally, with a Windows-only expectation in `TestCheckMisconfigurationDatabasePermissions`. Awaiting a Windows run. CI #46 (`0d3b91a`, run 36353006078) is green on ubuntu, macOS, and Windows. The Windows-only expectation ran there. | Done |
| P1-6 | Define `--quiet` with `--json` behavior: make them mutually exclusive (Cobra `MarkFlagsMutuallyExclusive`), or let quiet win. | The documented behavior is covered by a test and the README is updated. Done: `--quiet` wins over `--json` (chosen so the combination is not an error). README updated. | Done |
| P1-7 | Keep outcome messages single-line. `service-uptime` currently embeds the full combined `systemctl` output. | A test with multi-line fake output yields a single-line message. Done: `firstLine` for `systemctl` output, covered by a multi-line test. The `docker-status` success message can still be multi-line (see `notes.md`). | Done |
| P1-8 | Make checks testable without host state: inject a command runner (`exec` wrapper) and file readers. Add fixture-based parser tests for `/proc/meminfo`, `/proc/loadavg`, and `/proc/uptime`, and fake-runner tests for docker, kubectl, and systemctl paths. Stop `TestRunChecksDefaultsToAllChecks` from executing real CLIs. | No test shells out to real `docker`, `kubectl`, or `systemctl`. `internal` coverage is above 80%. Done: `lookPath`/`runCommand` seams on `CheckOptions`, `fakeToolOptions`, and `parseMeminfo`/`parseLoadAverage`/`parseUptime` fixture tests. `internal` coverage 87.7%. | Done |
| P1-9 | Use a distinct exit code for operational errors (Q-004: approved). Proposed value `3`, for any error that prevents a complete run: bad flags, unknown `--only` key, database failure. Builds on P1-1. | Tests cover exit code 3 for each error class. README exit-code section, `maint.md` contracts, and `history.md` updated. Smoke steps already tolerate only 0 and 1, so they catch 3. Done: `ExitCodeError = 3` and `ExitCode`. `TestRunExitCodes` covers unknown flag, unknown command or subcommand, extra argument, unknown check, invalid and missing job id. `TestRunDatabaseInitFailureIsOperationalError` covers DB init failure. | Done |
| P1-10 | Move the default database to a per-user data directory (Q-002: approved) and open it only for commands that need it, not `--help`, `--version`, `completion`, or `check run --no-save`. Proposed locations: Linux `$XDG_DATA_HOME/salus/salus.db` (fallback `~/.local/share/salus/salus.db`), macOS `~/Library/Application Support/salus/salus.db`, Windows `%LocalAppData%\salus\salus.db`. `SALUS_DB_PATH` keeps overriding (the container image sets it). Create the directory `0700` and the file `0600` (SEC-004). Document that an existing `./salus.db` is no longer read by default. | Running `salus --help` in an empty directory creates no file. Per-OS path resolution is unit-tested. README configuration is updated. Implemented 2026-09-27 (after v1.0.0): `internal.DatabasePath`/`DefaultDatabasePath` (with an `os/user` fallback when `HOME` is unset), a `DatabaseOpener` passed to commands, `main.run` opening at most once, and 0700/0600 creation. `TestRunOpensDatabaseOnlyWhenNeeded` and `TestRunUsesPerUserDefaultDatabase` cover it. Upgrade notes are in the README. Merged in `753252e`: CI #48, Docker #14, and Security #53 green. | Done |
| P1-11 | Reduce error noise. Cobra's error and usage printing is silenced at the root, and `main.run` writes `Error: …` plus `Run '<cmd> --help' for usage.` to stderr. GORM's logger is silenced. Group commands reject unknown subcommands, and leaf commands declare `Args`. | Errors never reach stdout (asserted for every error case in `TestRunExitCodes`). `jobs show 999` prints one error line to stderr and exits 3. `salus check rn` exits 3 with a suggestion. The acceptance criterion changed from "flag errors still show usage" to a `--help` hint for every error. This also fixes a regression found in review: usage text went to stdout once `run` set the output writer. | Done |

## Phase 2: Security hardening

Order by risk and effort. Update `cybersec.md` status as each item moves.

| ID | Work | Depends on | Status |
|---|---|---|---|
| P2-1 | SEC-001: validate `--service` and pass `--` to `systemctl`. | P1-8 (runner injection makes the test clean) Implemented 2026-09-27. See SEC-001 resolution. Merged in `753252e`: CI #48, Docker #14, and Security #53 green. SEC-001 Closed. | Done |
| P2-2 | SEC-004: create the DB file with `0600`, check the effective DB path in `misconfig` (including read bits), and bound message length. | P1-3, P1-5 Implemented 2026-09-27 together with P1-10. See SEC-004 resolution. Merged in `753252e`: CI #48, Docker #14, and Security #53 green. SEC-004 Closed. | Done |
| P2-3 | SEC-002 and SEC-003: non-root container user, digest-pinned base images, `.dockerignore`. | P0-4 Implemented 2026-09-27: UID 10001 `salus` user owns `/app/data` (0700), both stages are digest-pinned on Alpine 3.24, and `.dockerignore` was added. The non-root assertion is in the P2-4 workflow patch. The local `.env` context check for SEC-003 is still open (no Docker daemon was available). | Awaiting CI |
| P2-4 | SEC-005: `govulncheck` in CI and `.github/dependabot.yml` (gomod, github-actions, docker). gosec stays non-blocking (Q-009), and that policy is recorded in `maint.md`. | none Delivered as `salus-p24-workflows.patch`, because the remote session cannot write `.github/`: a `govulncheck@v1.8.0` job in `security.yml`, `.github/dependabot.yml` (gomod, github-actions, docker), and new `docker.yml` assertions. Checked with actionlint and simulated steps; `git apply --check` passes against `753252e`. | Awaiting merge |
| P2-5 | SEC-007: README container guidance. | P4-1 Done ahead of P4-1: the README Docker section rewritten (unsupported in-container checks, no socket mount, bind-mount ownership, 1.0.x volume upgrade). CI assertion in the P2-4 patch. | Awaiting CI |
| P2-6 | SEC-006: release provenance or signing. | P0-3 | Proposed |
| P2-7 | SEC-008: build with the latest Go patch release. `go.mod` `go 1.26.0` → `go 1.26.8`, because CI and CD install exactly the `go.mod` version. Then cut a release so users get a patched binary. | P2-4 (`govulncheck`) | Awaiting CI |

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
| P3-7 | macOS and Windows resource checks (Q-005: approved). macOS: `statfs`, sysctl (`hw.memsize`, `vm.loadavg`, `kern.boottime`, page counts). Windows: kernel32 (`GetDiskFreeSpaceExW`, `GlobalMemoryStatusEx`, `GetTickCount64`). Windows has no load average, so CPU load needs its own definition (for example utilization sampled with `GetSystemTimes`), recorded in `maint.md`. | Try the standard library `syscall` package first. Adopt `golang.org/x/sys` only if it proves insufficient, and record why. Then update `NOTICE`. Widen the `//go:build linux` constraint on `internal/health-thresholds.go` to the new platforms. Unsupported-platform stubs remain for other OSes. | Ready |
| P3-8 | Multiple services in one run (for example a repeatable `--service`). | Output and storage use one outcome per key today. Needs a design for per-service keys. | Proposed |

## Phase 4: Documentation and developer experience

| ID | Work | Status |
|---|---|---|
| P4-1 | Align the README with the `AGENTS.md` section order: prerequisites (Go 1.26, C toolchain for CGO), build from source, configuration (`SALUS_DB_PATH`), exit codes, testing and quality checks, project structure, and container caveats (SEC-007). The install section was already updated for archives in Phase 0 (Q-003). Add macOS Gatekeeper guidance for the unsigned binaries after confirming the behavior on a Mac (see SEC-006). | Proposed |
| P4-2 | Add Makefile targets `build`, `test`, `vet`, `lint`, `fmt`, and `cover`, keeping the existing release targets unchanged. | Proposed |
| P4-3 | Add an explicit `.golangci.yml` so the linter set does not drift with golangci-lint defaults. | Proposed |
| P4-4 | Add `SECURITY.md` with a vulnerability reporting channel. Needs maintainer input on the channel. | Proposed |
| P4-5 | Resolve `.idea/` handling (Q-007). Add issue and PR templates that match `CONTRIBUTING.md`'s issue-first rule. | Proposed |
| P4-6 | Dockerfile cleanup: fix the stale "rete.db" and "TUI" comments, and use `TARGETARCH` instead of a hard-coded `GOARCH=amd64`. Check whether `sqlite-libs` is needed at runtime (go-sqlite3 bundles SQLite unless built with the `libsqlite3` tag) using `ldd` before removing it. Stale comments and `TARGETARCH` done 2026-09-27 with P2-3. Update 2026-09-27: `sqlite-libs` and `ca-certificates` removed; unneeded per the go-sqlite3 source. The Docker smoke tests confirm the binary still runs. | Awaiting CI |

## Phase 5: Structure (optional, needs explicit approval)

| ID | Work | Status |
|---|---|---|
| P5-1 | Once the package grows, split `internal` into focused packages (for example `internal/checks`, `internal/store`, `internal/report`, `internal/cli`). Rename hyphenated files only as part of that move. | Proposed |
| P5-2 | Remove the empty TUI placeholders `internal/logic-tui.go` and `internal/ui-form.go` (Q-001: CLI only). Fix the Dockerfile "TUI" comment with P4-6. | Ready |

## Recommended sequence

1. **M1, green pipeline:** Done. CI, Docker, and Security are green on
   `0d3b91a`, and the CD workflow (P0-3) published v1.0.0 from `231487a`.
2. **M2, testable core:** Done. P1-1 to P1-9 and P1-11 shipped in `0d3b91a`, and CI
   is green on all three operating systems. P1-10 is implemented and awaiting CI.
3. **M3, hardened:** P2-1 and P2-2 are done (SEC-001 and SEC-004 closed). P2-3 and
   P2-5 are implemented and awaiting the Docker workflow. P2-4 is delivered as a
   workflow patch. After merge, confirm the Docker, Security (`govulncheck`), and
   Dependabot results, then close SEC-002, SEC-003, SEC-005, and SEC-007.
4. **M4, release hardening:** v1.0.0 was released on 2026-09-27 (before M3).
   Ship P1-10, P2-1, P2-2, P2-3, and P2-7 as the next release with the README
   upgrade notes (a minor or major bump is the maintainer's call; see
   `notes.md`). The image now runs as UID 10001, and 1.0.x Docker volumes need
   the documented one-time `chown`. Because v1.0.0 binaries were built with Go
   1.26.0 (SEC-008), consider also a v1.0.1 from the v1.0.0 tag with only the
   `go.mod` bump, for users who cannot take the behavior changes yet. Then
   P4-1, P4-2, P2-6, and P3-1.
5. **M5, depth:** P3-2 through P3-6, then P3-7 and P3-8 per decisions.
