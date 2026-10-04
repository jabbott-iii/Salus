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

## 2026-09-27: v1.0.0 released

- Change: Tagged `v1.0.0` at `231487a`. The CD workflow published six
  archives (`salus_{linux,darwin}_{amd64,arm64}.tar.gz`,
  `salus_windows_{amd64,arm64}.zip`) and `checksums.txt` as a GitHub Release.
  CI, Docker, and Security workflows were green on the preceding code commit
  `0d3b91a`.
- Files: none (tag and release only)
- Reason / reference: First release. It also confirms plan item P0-3 (CD
  workflow).

## 2026-09-27: Per-user database location, lazy opening, SEC-001 and SEC-004 remediation

- Change:
  - The default database moved from `./salus.db` to a per-user path
    (`$XDG_DATA_HOME/salus` or `~/.local/share/salus` on Linux and Unix,
    `~/Library/Application Support/salus` on macOS, `%LOCALAPPDATA%\salus`
    on Windows). `SALUS_DB_PATH` still overrides it.
  - Commands open the database lazily through a `DatabaseOpener`, so `--help`,
    `--version`, `completion`, and `check run --no-save` never create one.
  - New database files are created with mode `0600` and new directories with
    `0700`. Existing files are untouched, and read-only databases still open.
  - `misconfig` checks the effective database file for any group/other
    access.
  - Stored messages are capped at 1024 bytes.
  - `--service` values are validated as systemd unit names and passed after
    `--`.
  - `check run` validates `--only` and opens storage before running checks.
  - Removed `database_path.go` and `database_path_test.go` from package
    `main`; the logic moved to `internal/database-path.go`.
- Files: `main.go`, `main_test.go`, `version.go`, `database_path.go`
  (deleted), `database_path_test.go` (deleted), `internal/database-path.go`,
  `internal/database-path_test.go`, `internal/database.go`,
  `internal/health.go`, `internal/checks_test.go`, `internal/logic-cli.go`,
  `internal/logic-cli_test.go`, `internal/scan-store.go`,
  `internal/scan-store_test.go`, `README.md`, `CONTRIBUTING.md`,
  `intel/maint.md`, `intel/map.md`, `intel/notes.md`, `intel/plan.md`,
  `intel/cybersec.md`
- Reason / reference: Plan items P1-10 (Q-002), P2-1 (SEC-001), and P2-2
  (SEC-004).
  - An independent review before writing caught two defects, both fixed with
    tests: `?` parameters in `SALUS_DB_PATH` created a stray file and made the
    permission check read the wrong file, and opening with write access broke
    read-only databases.
  - This is a user-visible change after v1.0.0; see the README section
    "Upgrading from 1.0.x".
  - Uncommitted at the time of writing.

## 2026-09-27: Container hardening, vulnerability scanning, and update automation (P2-3 to P2-5)

- Change:
  - Closed SEC-001 and SEC-004 after `753252e` passed CI #48, Docker #14,
    and Security #53.
  - `Dockerfile`:
    - The runtime runs as `salus` (UID/GID 10001), which owns `/app/data`
      (0700).
    - Both stages are pinned by digest, and the runtime moved from
      `alpine:3.22` to `alpine:3.24` to match the builder's Alpine release.
    - The build honours `TARGETOS`/`TARGETARCH`, and the stale comments are
      fixed.
  - Added `.dockerignore`.
  - README Docker section:
    - Documents the non-root user and bind-mount ownership.
    - Says in-container Docker and Kubernetes checks are unsupported and warns
      against mounting the Docker socket.
    - Adds a 1.0.x volume `chown` step to "Upgrading from 1.0.x".
  - Workflow changes delivered as `salus-p24-workflows.patch`: a
    `govulncheck` job in `security.yml`, `.github/dependabot.yml` (gomod,
    github-actions, docker), and `docker.yml` assertions for the non-root
    user and in-container WARN results.
- Files: `Dockerfile`, `.dockerignore`, `README.md`, `intel/cybersec.md`,
  `intel/plan.md`, `intel/maint.md`, `intel/map.md`, `intel/notes.md`; via
  patch: `.github/workflows/security.yml`, `.github/workflows/docker.yml`,
  `.github/dependabot.yml`
- Reason / reference: Plan items P2-3 (SEC-002, SEC-003), P2-4 (SEC-005),
  and P2-5 (SEC-007).
  - The image was not built locally: no Docker daemon or registry access was
    available. The Docker workflow is the first build.
  - Uncommitted at the time of writing.

## 2026-09-27: Go directive raised to 1.26.8 (SEC-008); container review fixes

- Change:
  - `go.mod` `go 1.26.0` → `go 1.26.8`. CI and CD install exactly the
    `go.mod` version, so v1.0.0 release archives were built with Go 1.26.0
    and lack later standard-library security fixes (confirmed in the CI log
    for run 36367840814).
  - The Docker runtime stage no longer installs `sqlite-libs` or
    `ca-certificates` (both unused).
  - Dependabot's docker updates are limited to digest and patch bumps,
    grouped into one PR, so builder and runtime stay on the same Alpine
    release.
  - `.dockerignore` patterns now also match nested files.
  - The README 1.0.x volume fix-up command now also works when the database
    file is missing, and it sets the directory to 0700.
  - `docker.yml` asserts UID 10001 exactly.
- Files: `go.mod`, `Dockerfile`, `.dockerignore`, `README.md`,
  `CONTRIBUTING.md`, `intel/cybersec.md`, `intel/plan.md`, `intel/maint.md`,
  `intel/map.md`, `intel/notes.md`; via patch: `.github/dependabot.yml`,
  `.github/workflows/docker.yml`
- Reason / reference: An independent review of P2-3 and P2-4 flagged that the
  new `govulncheck` job would likely fail on Go 1.26.0. Verifying that in the
  CI log turned up SEC-008. A new release is recommended (`plan.md` M4).
  Uncommitted at the time of writing.

## 2026-09-27: v1.0.1 released

- Change: Tagged `v1.0.1` at `b66694a` on `main`. CD #2 published six
  archives plus `checksums.txt` as an immutable GitHub Release with a release
  attestation. v1.0.1 carries everything since v1.0.0 (`753252e` and
  `b66694a`):
  - The per-user database location with lazy opening (P1-10).
  - SEC-001 and SEC-004.
  - The non-root, digest-pinned image with `.dockerignore`.
  - `govulncheck` and Dependabot.
  - Go 1.26.8 (SEC-008).

  CI #49, Docker #15, and Security #54 were green on `b66694a`.
- Files: none (tag and release only)
- Reason / reference: Security rebuild for SEC-008. It also shipped the
  behavior changes described in the README, "Upgrading from 1.0.0".

## 2026-09-27: v1.0.1 validation recorded; SEC-002, SEC-007, SEC-008 closed

- Change:
  - Recorded the GitHub Actions evidence for `b66694a` and v1.0.1 in
    `intel/cybersec.md`:
    - SEC-002, SEC-007, and SEC-008 are Closed.
    - SEC-003 and SEC-005 remain In Progress, each pending one
      maintainer-only check. The steps are documented.
    - SEC-006 notes the immutable-release attestation.
  - Plan updates:
    - P2-5, P2-7, and P4-6 are Done.
    - P2-3 and P2-4 are Awaiting maintainer check, a new status value.
    - Added P2-8 (GitHub Actions maintenance).
  - Added open questions Q-010 (release attestation versus build provenance)
    and Q-011 (runner pinning).
  - Because v1.0.1 already carries the behavior changes, the README
    "Upgrading from 1.0.x" section is now "Upgrading from 1.0.0", and 1.0.x
    wording in `maint.md` and `notes.md` now says 1.0.0. Earlier history
    entries keep their original wording.
  - `maint.md` section 7 records which actions must move together.
  - `.github/dependabot.yml` groups the `github/codeql-action` sub-actions
    and the artifact actions. This is delivered as
    `salus-p28-dependabot.patch`, because the remote session cannot write
    `.github/`.
- Files: `README.md`, `intel/cybersec.md`, `intel/plan.md`, `intel/notes.md`,
  `intel/maint.md`, `intel/history.md`; via patch: `.github/dependabot.yml`
- Reason / reference: Validation of the v1.0.1 release (plan M3 and M4), and
  the first Dependabot run.
  - Dependabot split the CodeQL Action v4 bump across #15 and #17, and #17
    fails CodeQL.
  - The run annotations flag three dated deprecations: Node 20 actions,
    CodeQL Action v3 (December 2026), and the `ubuntu-latest` move to
    Ubuntu 26 (2026-10-19).
  - Uncommitted at the time of writing.

## 2026-09-27: Threshold flags, Makefile targets, README restructure, CD provenance and runner pin

- Change:
  - `check run` gained `--disk-warn`, `--disk-fail`, `--mem-warn`,
    `--mem-fail`, `--load-warn`, `--load-fail` (percentages), and `--timeout`
    (P3-1). Invalid values (NaN, infinite, zero, or negative; disk or memory
    above 100; warn not below fail; a timeout that is not positive) exit `3`
    before the database opens or any check runs. This is additive: the
    defaults are unchanged (80/90, 80/90, 80/100, 3s). They moved from
    `internal/health-thresholds.go` (Linux-only) to `internal/health.go`.
  - `Makefile`: new `build`, `test`, `vet`, `lint` (three GOOS targets),
    `fmt`, and `cover` targets, with the release targets unchanged (P4-2).
    `.gitignore` now ignores the `/salus` build output.
  - `README.md` follows the `AGENTS.md` section order: use cases,
    prerequisites, build from source, provenance verification, exit codes,
    testing, and project structure were added (P4-1). `CONTRIBUTING.md` was
    aligned with it.
  - `.github/workflows/cd.yml`:
    - An `actions/attest` v4.2.2 step signs build provenance for every archive
      in `checksums.txt` (P2-6, Q-010). The `release` job gains `id-token:
      write` and `attestations: write`.
    - The linux/amd64 build and the release job are pinned to `ubuntu-24.04`
      (Q-011).
    - `softprops/action-gh-release` moved from v2.6.2 to v3.0.3 (Node 24).
  - Recorded the Q-010 and Q-011 decisions and the P2-8 status. The Dependabot
    grouping from `78db94e` produced #18 and #19; #13, #20, and #21 were
    reviewed. Every action pin was verified against its upstream tag.
    SEC-006 is In Progress. SEC-005 gained supporting evidence from a local
    run of the vulnerable fixture.
- Files: `internal/health.go`, `internal/health-thresholds.go`,
  `internal/logic-cli.go`, `internal/logic-cli_test.go`, `main_test.go`,
  `Makefile`, `.gitignore`, `.github/workflows/cd.yml`, `README.md`,
  `CONTRIBUTING.md`, `intel/maint.md`, `intel/map.md`, `intel/cybersec.md`,
  `intel/notes.md`, `intel/plan.md`, `intel/history.md`
- Reason / reference: the plan M4 sequence (P2-8, P4-1, P4-2, P2-6, P3-1).
  - Validation run locally on Linux, all passing or clean:
    - `gofmt -s -l`, `go vet`, and `go test` with and without `-race`.
    - golangci-lint v2.13.2 for linux, darwin, and windows.
    - actionlint 1.7.12, also on a three-way merge of the local changes with
      Dependabot PRs #13 and #18 to #21.
    - govulncheck v1.8.0.
    - Every new `make` target.
  - Mutation checks: all 11 targeted mutations of the new flag wiring and
    validation were caught by the tests.
  - Not run: GitHub Actions for these changes; the CD `workflow_dispatch`
    check for #19 (the token lacks `actions: write`); macOS and Windows test
    execution; Docker.
  - Uncommitted at the time of writing.

## 2026-09-27: Review fixes to the provenance, Makefile, and threshold changes

- Change: An independent review of the previous entry's uncommitted changes
  led to these corrections before commit:
  - `cd.yml`: the release job is split in two. `package` runs only
    first-party actions. It packages, checksums, attests, and uploads the
    archives, and it alone has `id-token: write` and `attestations: write`.
    `release` downloads the archives and publishes them with
    `contents: write` and `softprops/action-gh-release`. A compromised
    third-party action can no longer sign provenance. This is now security
    requirement 10 in `intel/cybersec.md`.
  - README: the provenance check pins `--signer-workflow` and
    `--source-ref refs/tags/<tag>` and requires GitHub CLI 2.97 or newer,
    because `--repo` alone accepts any workflow and ref in the repository.
  - `Makefile`: `make lint` sets `GOOS=linux` explicitly, so a macOS host also
    lints the Linux build.
  - Tests: two more `check run` cases (the 100 limit, and
    `--mem-fail 100.5`) catch the two mutations that had survived.
  - Documentation wording:
    - the exit code on macOS and Windows;
    - `--no-save`;
    - what 100% load means;
    - the scope of write permissions in `maint.md`;
    - the `check run` sequence diagram in `map.md`.
- Files: `.github/workflows/cd.yml`, `Makefile`, `internal/logic-cli.go`
  (comment only), `internal/logic-cli_test.go`, `README.md`,
  `CONTRIBUTING.md`, `intel/maint.md`, `intel/map.md`, `intel/cybersec.md`,
  `intel/plan.md`, `intel/history.md`
- Reason / reference: the independent review of 2026-09-27. The previous
  entry's statements that the `release` job gains the signing permissions,
  and that 11 mutations were checked, describe the state before these fixes.
  - Validation rerun on the final state, all passing or clean:
    - `gofmt`, `go vet`, and `go test` (also with `-race -shuffle=on`).
    - `make lint` for linux, darwin, and windows.
    - actionlint 1.7.12, also on a three-way merge with Dependabot PRs #13 and
      #18 to #21.
    - govulncheck v1.8.0.
    - 13 of 13 targeted mutations caught.
  - Uncommitted at the time of writing.

## 2026-09-28: v1.0.2 released with build provenance; Dependabot action updates merged; SEC-006 closed

- Change:
  - The changes in the two previous entries were committed as `08b2faa` and
    tagged `v1.0.2`. CD #3 (36397627324) did the following:
    - built the six archives, with the linux/amd64 build and both release
      jobs on `ubuntu-24.04`;
    - attested them in the `package` job (6 subjects, Rekor log index
      2981647855);
    - published an immutable GitHub Release.
  - CI #69, Security #74, and Docker #26 were green on `08b2faa`, and the new
    `check run` tests passed on Linux, macOS, and Windows.
  - Dependabot PRs #18, #20, #21, #13, and #19 were merged after the tag
    (`2fd2496`). CI #76, Security #81, and Docker #32 there show no Node 20 or
    CodeQL v3 annotations. v1.0.2 itself was built with the earlier
    `checkout`, `setup-go`, and binary-artifact pins.
  - The v1.0.2 release notes were edited to describe the threshold flags, the
    provenance check, and the 1.0.0 upgrade steps. The seven release assets
    are unchanged.
  - SEC-006 closed. The documented `gh attestation verify` command, run with
    GitHub CLI 2.101.0, passes for `salus_linux_amd64.tar.gz`. It fails for a
    modified copy and for `--source-ref refs/heads/main`. The README now
    notes that the command needs a GitHub login.
  - Plan: P2-6, P3-1, P4-1, and P4-2 are Done. P2-8 awaits one CD run at
    `2fd2496` or later.
- Files: none for the release. This update: `README.md`,
  `intel/cybersec.md`, `intel/plan.md`, `intel/notes.md`, `intel/maint.md`,
  `intel/map.md`, `intel/history.md`
- Reason / reference: plan milestone M4, step 5, and the SEC-006 validation.
  - The maintainer approved the archive download and the release-notes edit
    for this session.
  - Uncommitted at the time of writing.

## 2026-09-28: M5 check depth (P3-2 to P3-6), TUI placeholders removed, SEC-009 fixed; P2-8 closed

- Change:
  - P2-8 closed by maintainer decision, without a manual CD run on `main`.
    The next tag is the first CD run with the updated artifact pins.
  - `docker-status` (P3-2) reports WARN for unhealthy or restarting
    containers, naming up to five. The Docker CLI's `WARNING` lines are no
    longer taken as the server version or as container names.
  - `kubernetes-status` (P3-3) checks node readiness:
    - WARN when some nodes are NotReady;
    - FAIL when no node is Ready;
    - PASS with a note when listing nodes is forbidden.

    The new `check run --kube-context` flag is validated and passed as one
    `--context=<name>` argument.
  - `misconfig` (P3-4) runs five rules with stable ids: `home-unset`,
    `db-permissions`, `kubeconfig-permissions`,
    `docker-socket-permissions`, and `path-world-writable`. Each problem is
    reported as `<id>: <details>`.
  - `jobs list --json` and `jobs show --json` (P3-5). Job results use the
    `check run --json` shape.
  - `jobs prune --older-than <age> [--dry-run]` (P3-6) deletes old jobs and
    their results in one transaction, comparing times with `julianday`.
  - Removed the empty `internal/logic-tui.go` and `internal/ui-form.go`
    (P5-2, Q-001).
  - SEC-009 (new, fixed): check messages had embedded external tool output
    unfiltered. `RunChecks` and `jobs show` now replace control characters.
  - The maintainer decided the severities for P3-2 (WARN) and P3-3 (graded).
    The README gains a per-check PASS/WARN/FAIL table, the misconfig rule ids,
    and an "Upgrading from 1.0.2" section for the changes that can raise exit
    codes.
- Files: `internal/health.go`, `internal/logic-cli.go`, `internal/report.go`,
  `internal/scan-store.go`, `internal/checks_test.go`,
  `internal/logic-cli_test.go`, `internal/scan-store_test.go`, `main_test.go`,
  `internal/logic-tui.go` (deleted), `internal/ui-form.go` (deleted),
  `README.md`, `intel/maint.md`, `intel/map.md`, `intel/cybersec.md`,
  `intel/notes.md`, `intel/plan.md`, `intel/history.md`
- Reason / reference: plan milestone M5 and P5-2. SEC-009 was found while
  extending the threat model for the new tool output.
  - Validated locally on Linux; see the completion report for the commands.
  - macOS and Windows execution is left to CI.
  - Uncommitted at the time of writing.

## 2026-09-28: Review fixes to M5 and SEC-009 (uncommitted)

- Change: An independent review of the previous entry's uncommitted changes
  led to these corrections before commit:
  - `kubernetes-status`: a Forbidden answer from `kubectl cluster-info` now
    counts as reachable. `cluster-info` lists kube-system Services, so
    namespace-scoped users previously got FAIL with kubectl's "To further
    debug" hint as the message.
  - New `errorLine`: failure messages skip Docker CLI `WARNING` lines,
    kubectl log lines, and that hint.
  - `--kube-context` accepts any valid UTF-8 name of at most 253 bytes that
    does not start with `-` and has no control characters. It is still passed
    as one `--context=<name>` argument.
  - The `misconfig` permission rules skip WSL drvfs paths (`syntheticModes`,
    new `internal/health-mounts_linux.go` and `_other.go`). WSL appends the
    Windows `PATH`, whose made-up 0777 modes would otherwise warn on every
    default WSL host.
  - SEC-009: `jobs show --json` now also sanitizes stored messages, because
    JSON does not escape DEL or C1 characters. The earlier claim that JSON
    was unaffected is corrected in `intel/cybersec.md`. SEC-009 remains In
    Progress until CI runs; the previous entry's "fixed" means fixed in the
    working tree.
  - `DOCKER_HOST=unix://` without a path now means the default socket.
  - New tests cover these cases and the two mutations the review found
    surviving.
  - Documentation:
    - the README notes the WSL skip, `--limit 0`, and that `jobs prune`
      creates the database;
    - the `notes.md` gosec baseline records two new G703 false positives
      (`os.Stat` on paths from the user's own environment), to be triaged in
      Code Scanning;
    - the `notes.md` timeout wording is corrected.
- Files: `internal/health.go`, `internal/health-mounts_linux.go` (new),
  `internal/health-mounts_other.go` (new),
  `internal/health-mounts_linux_test.go` (new), `internal/report.go`,
  `internal/checks_test.go`, `internal/logic-cli_test.go`, `README.md`,
  `intel/maint.md`, `intel/map.md`, `intel/cybersec.md`, `intel/notes.md`,
  `intel/plan.md`, `intel/history.md`
- Reason / reference: the independent review of 2026-09-28. The whitespace
  change in `AGENTS.md` in the same working tree was made by the maintainer,
  not by this work.
  - Uncommitted at the time of writing.

## 2026-10-03: M6 — per-target results, monitoring outputs, new checks, run history (uncommitted)

- Change:
  - Per-target results (P6-1): `CheckOutcome` and `ScanResult` gain `target`,
    `value`, and `unit`. `RunChecks` runs `disk-space`, `disk-inodes`,
    `service-uptime`, and `cert-expiry` once per `--disk-path`, `--service`,
    or `--cert` value (`checkTargets`), and runs a repeated `--only` key once.
    Resource checks report their percentage, host uptime its seconds, and the
    count-based checks their counts. `AutoMigrate` adds the three columns to
    existing databases. Supersedes P3-8.
  - `check run` output and control (P6-2 to P6-4): `--format
    text|json|nagios|prometheus|junit`, `--output` (validated before the run,
    written atomically, symlinks rejected, mode and group of an existing file
    kept), `--fail-on warn|fail`, and `--retain <age>` (prunes after the
    report is written, never the current run).
  - New checks, appended after `misconfig` in `AllCheckKeys` and run by
    default (Q-012): `disk-inodes`, `kubernetes-pods` (with
    `--kube-namespace`), `systemd-failed`, `time-sync`, and `cert-expiry`
    (only with `--cert`; `--cert-warn-days`). New catalog category
    "Certificates".
  - `kubernetes-status` reports nodes under Memory, Disk, or PID pressure as
    WARN (Q-014).
  - `misconfig` rules `docker-tcp-insecure`, `sshd-root-login`, and
    `sshd-password-auth` (effective value, Q-013), with an sshd_config
    reader and the new `SALUS_SSHD_CONFIG` override.
  - `jobs diff` and `jobs stats` (P6-11, P6-12). `ListScanJobs` now orders by
    `julianday(started_at)`.
  - Seed descriptions for `disk-space`, `docker-status`, `kubernetes-status`,
    and `service-uptime` updated; existing databases keep their old text
    (insert-only catalog).
  - Documentation: README (features, use cases, flags, check and rule tables,
    output formats, jobs commands, exit codes, configuration, "Upgrading from
    1.0.2", containerization, structure), CONTRIBUTING (contracts, new-check
    checklist, non-root tests), and `maint.md`, `map.md`, `cybersec.md`
    (threat model, requirements 12 and 13, controls; no new issue),
    `notes.md` (Q-012 to Q-014, behavior notes, expected gosec findings),
    and `plan.md` (Phase 6).
- Files:
  - New: `internal/health-certs.go`, `internal/health-pods.go`,
    `internal/health-sshd.go`, `internal/health-systemd.go`,
    `internal/report-formats.go`, `internal/report-files_unix.go`,
    `internal/report-files_windows.go`, `internal/scan-history.go`, and tests
    `internal/health-certs_test.go`, `internal/health-pods_test.go`,
    `internal/health-sshd_test.go`, `internal/health-sshd_posix_test.go`,
    `internal/health-systemd_test.go`, `internal/report-formats_test.go`,
    `internal/scan-history_test.go`.
  - Changed: `internal/health.go`, `internal/health-resources_linux.go`,
    `internal/health-resources_other.go`, `internal/health-thresholds.go`,
    `internal/logic-cli.go`, `internal/report.go`, `internal/scan-store.go`,
    `internal/database.go`, `internal/seed.go`, their tests,
    `main_test.go`, `README.md`, `CONTRIBUTING.md`, and `intel/*.md` except
    `golang.md`.
  - Unchanged: `go.mod`, `go.sum`, `NOTICE` (standard library only),
    workflows, `Dockerfile`, `Makefile`.
- Reason / reference: maintainer request of 2026-10-03 to plan and implement
  the recommended features (`plan.md` Phase 6; decisions Q-012 to Q-014).
  - Validated locally; see "M6 validation" in `plan.md` for the commands,
    results, and what was not run.
  - An independent review found no high-severity defects; its medium and low
    findings were fixed before handover (listed in `plan.md`).
  - Uncommitted at the time of writing.
