# Implementation Plan

Active implementation plans and follow-on work. Architecture rules are in
[`maint.md`](maint.md). Security items (`SEC-*`) are defined in
[`cybersec.md`](cybersec.md), and open questions (`Q-*`) in [`notes.md`](notes.md).

Last reviewed: 2026-09-28 (against `5827c8f` plus the uncommitted M5 and SEC-009
changes; v1.0.2 is at `08b2faa`). Decisions on Q-001 to Q-011 are recorded in
`notes.md`; the P3-2 and P3-3 severity decisions are in `maint.md` section 3.

Status values: `Proposed` (not started), `Ready` (decision made, can start),
`In Progress`, `Blocked`, `Awaiting merge` (delivered for the maintainer to
apply: a patch, uncommitted working-tree changes, or an open PR that needs
approval), `Awaiting CI` (implemented
and validated locally, waiting for a GitHub Actions run), `Awaiting maintainer
check` (merged and CI-validated; one validation step needs something only the
maintainer has, such as a Docker daemon or a branch push), `Done`.

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
| P2-3 | SEC-002 and SEC-003: non-root container user, digest-pinned base images, `.dockerignore`. | P0-4 Implemented 2026-09-27: UID 10001 `salus` user owns `/app/data` (0700), both stages are digest-pinned on Alpine 3.24, and `.dockerignore` was added. Merged in `b66694a` (v1.0.1). Docker #15 is green, with UID 10001 asserted, so SEC-002 is Closed. The `.dockerignore` passed a BuildKit-matcher check with decoy files. SEC-003 stays In Progress until the maintainer runs the four-command local `.env` check in `cybersec.md`. | Awaiting maintainer check |
| P2-4 | SEC-005: `govulncheck` in CI and `.github/dependabot.yml` (gomod, github-actions, docker). gosec stays non-blocking (Q-009), and that policy is recorded in `maint.md`. | none Delivered as `salus-p24-workflows.patch`, because the remote session cannot write `.github/`: a `govulncheck@v1.8.0` job in `security.yml`, `.github/dependabot.yml` (gomod, github-actions, docker), and new `docker.yml` assertions. Merged in `b66694a` (v1.0.1). Security #54 ran `govulncheck`, which found no vulnerabilities. Dependabot opened #13 to #17. Remaining for SEC-005: one failing `govulncheck` run on a throwaway branch (fixture in `cybersec.md`). | Awaiting maintainer check |
| P2-5 | SEC-007: README container guidance. | P4-1 Done ahead of P4-1: the README Docker section rewritten (unsupported in-container checks, no socket mount, bind-mount ownership, 1.0.0 volume upgrade). The CI assertion passed in Docker #15. The socket case was checked with the released binary. SEC-007 Closed. | Done |
| P2-6 | SEC-006: release provenance or signing. | P0-3 Note: v1.0.1 is an immutable GitHub release with a release attestation (`gh release verify-asset`). That covers tampering after publication, but not build provenance; see SEC-006. Decide whether to add build provenance (`actions/attest-build-provenance` in the `release` job) or accept the release attestation (Q-010). Q-010 decided 2026-09-27: add build provenance. Implemented (uncommitted) with `actions/attest` v4.2.2, the maintained successor (`attest-build-provenance` v4 is a wrapper around it). After an independent review, the CD release job was split in two. `package` runs only first-party actions: it packages, writes checksums, attests with `subject-checksums: dist/checksums.txt`, and uploads the archives. It alone has `id-token: write` and `attestations: write`. `release` publishes them with `contents: write` and softprops v3.0.3 (tags only). A compromised third-party action therefore cannot sign provenance (security requirement 10). The README documents the strict check (`--signer-workflow` and `--source-ref refs/tags/<tag>`, GitHub CLI 2.97 or newer), because `--repo` alone accepts any workflow and ref in the repository. Validated locally with actionlint (including a three-way merge with the open Dependabot PRs), a packaging and checksum-parser simulation, and the artifact actions' documented path behavior. Next: a manual CD run after merge, then verify an archive from the next release (see SEC-006). **Validated 2026-09-28 on v1.0.2:** CD #3 attested six subjects. The documented command passes for `salus_linux_amd64.tar.gz` and fails for a modified copy and for a wrong `--source-ref`. SEC-006 Closed. | Done |
| P2-7 | SEC-008: build with the latest Go patch release. `go.mod` `go 1.26.0` → `go 1.26.8`, because CI and CD install exactly the `go.mod` version. Then cut a release so users get a patched binary. | P2-4 (`govulncheck`) Released as v1.0.1 (CD #2). CI and CD logs show go1.26.8, and `go version -m` on the released linux/amd64 binary reports go1.26.8. SEC-008 Closed. | Done |
| P2-8 | GitHub Actions maintenance, surfaced by the first Dependabot run and v1.0.1 annotations. (a) Group coupled actions in `.github/dependabot.yml`: every `github/codeql-action` sub-action, and `actions/upload-artifact` with `actions/download-artifact`. Delivered as `salus-p28-dependabot.patch`. Today #15 and #17 bump single CodeQL sub-actions, and #17 fails CodeQL. (b) Move `github/codeql-action` to v4 as one group before its v3 deprecation in December 2026, per the CodeQL annotation. (c) Take the Node 24 majors of `actions/checkout` and `actions/setup-go`: v4.4.0 and v5.6.0 target Node 20, and runners already force them onto Node 24 (warning). (d) `upload-artifact` and `download-artifact` run only in `cd.yml`, so PR checks do not exercise #14 or #16. Run CD with `workflow_dispatch` on their branch first; the release step runs only for tags. (e) `ubuntu-latest` moves to Ubuntu 26 from 2026-10-19, per the runner annotation. Run CD with `workflow_dispatch` after that date and before the next tag, or pin `ubuntu-24.04` (Q-011). **Status 2026-09-27 (later):** (a) Done in `78db94e`: Dependabot closed #14 to #17 and opened the grouped #18 and #19. (b) #18 moves all four CodeQL sub-actions to v4.38.2 and passes Security, so it only needs approval and merge. (c) #21 (`checkout` v7.0.1) and #13 (`setup-go` v7.0.0) pass every check. #20 (`codecov-action` v7.1.1) is also needed, because Codecov v5 runs `actions/github-script` v7 (Node 20) internally. Release notes were reviewed, and setup-go v7 still installs Go 1.26.8. (d) Not run: this session's token cannot dispatch workflows (HTTP 403). The command is `gh workflow run cd.yml --ref dependabot/github_actions/artifact-actions-055219aa09`, or use the Actions tab. (e) Q-011 decided (pin CD only): CD's linux/amd64 build and its release job now use `ubuntu-24.04` (uncommitted). (f) New: `softprops/action-gh-release` v2.6.2 is Node 20 too, per the CD annotations. It was moved to v3.0.3 in the working tree, because Dependabot was at its five-PR limit. Every new pin was verified against its upstream tag. The five PRs and the local changes merge cleanly in a three-way simulation, and the result passes actionlint 1.7.12. | P2-4 Acceptance: one grouped CodeQL v4 PR passes Security (met by #18), and a `workflow_dispatch` CD run on the artifact-action branch packages six archives. A manual CD run on `main` after merging, before the next tag, is an equivalent check, because CD runs only for tags and manual dispatches. The Security, CI, CD, and Docker annotations no longer show the Node 20 warning. **Status 2026-09-28:**<br>• Local changes merged as `08b2faa`; #18, #20, #21, #13, and #19 merged (`2fd2496`).<br>• CI #76, Security #81, and Docker #32 on `2fd2496` have no Node 20 or CodeQL v3 annotations.<br>• CD #3 (v1.0.2) confirmed the `ubuntu-24.04` pins and a Node 24 softprops, and its v7/v8 `release-archives` artifact pair worked.<br>• v1.0.2 is tagged at `08b2faa`, before the Dependabot merges, so CD #3 still used the old `checkout`, `setup-go`, and binary-artifact pins, with Node 20 warnings.<br>• Remaining: one CD run at `2fd2496` or later (manual dispatch on `main`, or the next tag) to confirm the bumped build upload and package download, and CD annotations without Node 20.<br>• **Closed 2026-09-28 by maintainer decision**, without that CD run. The bumped pins (`upload-artifact` v7.0.1 for the binaries, and `download-artifact` v8.0.1 in pattern mode) first run in CD at the next tag. The same versions already ran in CD #3 for `release-archives`. | Done |
| P2-9 | SEC-009: replace control characters in check messages before they are printed or stored, and in stored messages that `jobs show` prints. | P3-2, P3-3 (more tool output in messages). Implemented 2026-09-28 (uncommitted): `sanitizeMessage` in `RunChecks` and in `jobs show`. Tests cover a hostile daemon's escape sequences and old stored rows. | Awaiting CI |

## Phase 3: Feature completeness

Scope comes from the project description (system health, Docker, Kubernetes,
memory, CPU, disk, service uptime, misconfiguration detection). Each item
needs tests without host dependence (P1-8) and README updates.

| ID | Work | Notes | Status |
|---|---|---|---|
| P3-1 | Expose thresholds and the command timeout as flags on `check run` (for example `--disk-warn`, `--disk-fail`, `--mem-warn`, `--mem-fail`, `--load-warn`, `--load-fail`, `--timeout`). Validate warn < fail and sane ranges. | `CheckOptions` already supports these fields. This is additive and non-breaking. Implemented 2026-09-27 (uncommitted). The flags are bound to `CheckOptions`, and the defaults moved to `health.go`. `validateLimits` requires finite values above 0, disk and memory values of at most 100, warn below fail, and a positive timeout. It returns exit 3 before the database opens. `newCheckRunCmdWith` injects the check runner for tests. Tests: `TestCheckRunPassesFlagValuesToChecks` (defaults, overrides, and the 100 limit), `TestCheckRunRejectsInvalidLimits` (12 cases), and a `TestRunExitCodes` case. All 13 targeted mutations were caught. That count includes the two the independent review found surviving (no memory cap, `>=` at the cap), which led to the 100-limit and `--mem-fail 100.5` cases. Tests (with and without `-race`) pass on Linux, and golangci-lint passes for linux, darwin, and windows. README updated. Merged in `08b2faa`. CI #69 is green on ubuntu, macOS, and Windows (the new tests ran on all three). Released in v1.0.2. | Done |
| P3-2 | Docker container health: report unhealthy or restarting containers, not only daemon reachability. | Matches the "Container Runtime" category description in `seed.go`. Decided 2026-09-28: WARN, naming the containers. Implemented (uncommitted): `docker ps` filtered with `health=unhealthy` and `--all status=restarting`. Parsing ignores Docker CLI `WARNING` lines, which also fixes the multi-line version message noted in `notes.md`. A failed listing gives WARN ("container health unknown"). | Awaiting CI |
| P3-3 | Kubernetes depth: node readiness, plus an optional `--kube-context`. | Keep `kubectl` as the integration. No client-go dependency without a decision. Decided 2026-09-28: graded. Some nodes NotReady gives WARN, no Ready node gives FAIL, and a Forbidden node listing stays PASS with a note. Implemented (uncommitted): `kubectl get nodes` with a JSONPath for the `Ready` condition. `--kube-context` is validated like `--service` and passed as the single argument `--context=<name>`. Review fixes: a Forbidden answer from `cluster-info` also counts as reachable, because namespace-scoped users cannot list kube-system Services. `errorLine` skips kubectl's log lines and its "To further debug" hint. `validKubeContext` now only rejects a leading `-`, control characters, invalid UTF-8, and names over 253 bytes. | Awaiting CI |
| P3-4 | Broader misconfiguration detection: kubeconfig permissions, Docker socket permissions, world-writable `PATH` entries, and DB file mode (with P2-2). | Every rule gets a stable identifier in the message and a test. Implemented (uncommitted) as `misconfigRules`, whose five ids are listed in `maint.md` section 3 and the README. The permission rules skip Windows. Test isolation now pins `KUBECONFIG`, `DOCKER_HOST`, and `PATH`. Review fix: the permission rules skip WSL drvfs paths (`syntheticModes`), whose modes are made up; otherwise every default WSL host would warn about the Windows `PATH`. | Awaiting CI |
| P3-5 | Machine-readable history: `jobs list --json` and `jobs show --json`. | Additive. Does not change the `check run --json` array shape. Implemented (uncommitted). A job object has `id`, `status`, `started_at`, `finished_at`, and `summary`. `jobs show` adds `results` in the `check run --json` shape. An empty list prints `[]`. | Awaiting CI |
| P3-6 | Job retention: a way to prune old jobs (for example `jobs prune --older-than 30d`). | Prevents unbounded DB growth under cron. Implemented (uncommitted): `jobs prune --older-than <days\|duration> [--dry-run]` and `PruneScanJobs`. One transaction deletes the results explicitly, uses a subquery, and compares times with `julianday` (see `notes.md`). | Awaiting CI |
| P3-7 | macOS and Windows resource checks (Q-005: approved). macOS: `statfs`, sysctl (`hw.memsize`, `vm.loadavg`, `kern.boottime`, page counts). Windows: kernel32 (`GetDiskFreeSpaceExW`, `GlobalMemoryStatusEx`, `GetTickCount64`). Windows has no load average, so CPU load needs its own definition (for example utilization sampled with `GetSystemTimes`), recorded in `maint.md`. | Try the standard library `syscall` package first. Adopt `golang.org/x/sys` only if it proves insufficient, and record why. Then update `NOTICE`. Widen the `//go:build linux` constraint on `internal/health-thresholds.go` to the new platforms. Unsupported-platform stubs remain for other OSes. | Ready |
| P3-8 | Multiple services in one run (for example a repeatable `--service`). | Output and storage use one outcome per key today. Needs a design for per-service keys. | Proposed |

## Phase 4: Documentation and developer experience

| ID | Work | Status |
|---|---|---|
| P4-1 | Align the README with the `AGENTS.md` section order: prerequisites (Go 1.26, C toolchain for CGO), build from source, configuration (`SALUS_DB_PATH`), exit codes, testing and quality checks, project structure, and container caveats (SEC-007). The install section was already updated for archives in Phase 0 (Q-003). Add macOS Gatekeeper guidance for the unsigned binaries after confirming the behavior on a Mac (see SEC-006). Done 2026-09-27 (uncommitted). The README follows the `AGENTS.md` order and adds use cases, prerequisites, build from source, `gh attestation verify`, an exit-code table, the new threshold flags, testing and quality checks, and the project structure. `CONTRIBUTING.md` was aligned (Make targets, exit code 3 as a contract, manual CD runs before tags). The Gatekeeper guidance moved to P4-7. Merged in `08b2faa` and released in v1.0.2. | Done |
| P4-2 | Add Makefile targets `build`, `test`, `vet`, `lint`, `fmt`, and `cover`, keeping the existing release targets unchanged. Done 2026-09-27 (uncommitted). Every target was run: `make lint` (golangci-lint with an explicit `GOOS=linux`, `darwin`, and `windows`, so a macOS host also lints Linux; this was a review fix, checked with a decoy unused Linux-only function; override the binary with `GOLANGCI_LINT=`) reports 0 issues. The release targets are unchanged, checked with the `check-version` guard and `make -n release`. `/salus` was added to `.gitignore`, so `make build` output stays untracked. Merged in `08b2faa` and released in v1.0.2. | Done |
| P4-3 | Add an explicit `.golangci.yml` so the linter set does not drift with golangci-lint defaults. | Proposed |
| P4-4 | Add `SECURITY.md` with a vulnerability reporting channel. Needs maintainer input on the channel. | Proposed |
| P4-5 | Resolve `.idea/` handling (Q-007). Add issue and PR templates that match `CONTRIBUTING.md`'s issue-first rule. | Proposed |
| P4-6 | Dockerfile cleanup: fix the stale "rete.db" and "TUI" comments, and use `TARGETARCH` instead of a hard-coded `GOARCH=amd64`. Check whether `sqlite-libs` is needed at runtime (go-sqlite3 bundles SQLite unless built with the `libsqlite3` tag) using `ldd` before removing it. Stale comments and `TARGETARCH` done 2026-09-27 with P2-3. Update 2026-09-27: `sqlite-libs` and `ca-certificates` removed; unneeded per the go-sqlite3 source. Docker #15 on `b66694a` is green, and the binary runs and persists to the volume without them. | Done |
| P4-7 | macOS Gatekeeper guidance for the unsigned release binaries (split from P4-1). On a Mac, confirm what happens when a binary extracted from a downloaded archive is run: whether the quarantine attribute blocks it, and whether `xattr -d com.apple.quarantine` is needed. Then document it in the README install section. SEC-006 covers provenance, not Apple code signing. | Blocked (needs a Mac) |

## Phase 5: Structure (optional, needs explicit approval)

| ID | Work | Status |
|---|---|---|
| P5-1 | Once the package grows, split `internal` into focused packages (for example `internal/checks`, `internal/store`, `internal/report`, `internal/cli`). Rename hyphenated files only as part of that move. | Proposed |
| P5-2 | Remove the empty TUI placeholders `internal/logic-tui.go` and `internal/ui-form.go` (Q-001: CLI only). Fix the Dockerfile "TUI" comment with P4-6. Done 2026-09-28 (uncommitted): both files deleted. Each held only the license header and `package internal`. | Awaiting CI |

## Recommended sequence

1. **M1, green pipeline:** Done. CI, Docker, and Security are green on
   `0d3b91a`, and the CD workflow (P0-3) published v1.0.0 from `231487a`.
2. **M2, testable core:** Done. P1-1 to P1-9 and P1-11 shipped in `0d3b91a`, and CI
   is green on all three operating systems. P1-10 shipped in `753252e`.
3. **M3, hardened:** Mostly done. SEC-001, SEC-002, SEC-004, SEC-007, and SEC-008
   are Closed. Two maintainer-only checks remain: the local `.env` build-context
   check (SEC-003) and one failing `govulncheck` run on a throwaway branch
   (SEC-005). The exact steps are in `cybersec.md`.
4. **M4, release hardening:** v1.0.0 was released on 2026-09-27 (before M3).
   v1.0.1 followed the same day from `b66694a` on `main`. It carries P1-10,
   P2-1 to P2-5, P2-7, and P4-6, not only the Go bump. Its behavior changes
   (per-user database location, `0600` warning, `--service` validation,
   non-root image) are documented in the README ("Upgrading from 1.0.0").
   However, the generated release notes contain only the changelog link, and
   release notes stay editable on an immutable release. Next: P2-8 first,
   because it has dates (the Ubuntu 26 runner switch on 2026-10-19 and the
   CodeQL Action v3 deprecation in December 2026). Then P4-1, P4-2, P2-6, and
   P3-1.
   **Update 2026-09-27 (later):** P4-1, P4-2, P2-6, P3-1, and the local part of
   P2-8 are implemented in the working tree (uncommitted). Maintainer steps, in
   order:
   1. Review, commit, and push the working tree. Pushing it first lets
      Dependabot rebase its PRs; CI then runs the P3-1 tests on all three
      operating systems.
   2. Approve and merge Dependabot #18, #20, #21, and #13. All checks are
      green.
   3. Validate #19 (artifact actions): run CD manually on its branch, confirm
      it packages six archives, and merge. Alternatively, merge it and run the
      check in step 4.
   4. Run CD manually on `main`. This exercises the `package` job
      (attestation), the `release` job's download, the artifact actions, and
      the pinned runners together. Confirm that no annotation mentions
      Node 20.
   5. Tag the next release. P3-1 adds flags, so v1.1.0 fits semantic
      versioning. Verify one archive with `gh attestation verify` and a
      modified copy (SEC-006), and consider linking "Upgrading from 1.0.0" in
      the release notes.
   6. Still open from M3: the SEC-003 and SEC-005 maintainer checks.

   **Update 2026-09-28:**
   - Steps 1, 2, and 5 are done.
   - The working tree was committed as `08b2faa` and all five Dependabot PRs
     were merged (`2fd2496`).
   - v1.0.2 was released from `08b2faa` with verified build provenance
     (SEC-006 Closed). Its release notes now describe the new flags, the
     provenance check, and the 1.0.0 upgrade steps.
   - #19 was merged without the separate branch run (step 3).
   - Step 4 is still open: run CD manually on `main`, from the Actions tab,
     because the agent's token cannot dispatch. That closes P2-8.
     *Update:* the maintainer closed P2-8 on 2026-09-28 without this run.
     The next tag is the first CD run with the updated pins.
   - Still open: SEC-003 and SEC-005. After that, M5.
5. **M5, depth:** P3-2 through P3-6, then P3-7 and P3-8 per decisions.
   P5-2 (removing the empty TUI placeholders) is Ready and small; it can go
   in any time.
   **Update 2026-09-28:** P3-2 to P3-6, P5-2, and P2-9 (SEC-009, found while
   extending the threat model) are implemented in the working tree.
   - They change results on some hosts. The README section "Upgrading from
     1.0.2" lists the changes, so the next release should be a minor version
     (v1.1.0).
   - Next: commit, let CI run on all three operating systems, and release.
     Then P3-7 (macOS and Windows resource checks), which is Ready, and P3-8,
     which needs a design for per-service keys.
