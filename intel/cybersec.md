# Security Requirements and Issue Tracking

Security requirements, identified issues, remediation items, and their status.
Rules for this file are in `AGENTS.md` ("Security Issue Tracking"): never
delete items, mark `Closed` only after remediation and validation, and never
regress a documented remediation.

Last reviewed: 2026-09-27 (against commit `753252e` plus uncommitted P2-3/P2-4
changes).

## Threat model summary

- **Deployment:** Local CLI run by an operator, a cron job, or a CI step; also
  shipped as a container image. No network listener and no HTTP API.
- **Trust boundaries:** Command-line flags and environment variables
  (`SALUS_DB_PATH`, `--service`, `--disk-path`, `--only`); output of external
  tools (`docker`, `kubectl`, `systemctl`); the SQLite file on disk; the
  CI/CD supply chain (actions, base images, Go modules, release artifacts).
- **Data handled:** Host health metadata: mount paths, service names, Docker
  server version, the first line of external tool errors (which can include
  cluster endpoints or host names), and timestamps. No credentials are read or
  stored by Salus itself.

## Security requirements

1. Never execute external commands through a shell. Pass arguments separately
   via `exec.CommandContext` with a timeout.
2. Validate every user-supplied value before it becomes an argument to an
   external command.
3. Use GORM query builders or parameterized queries only. Never concatenate
   input into SQL.
4. Do not store or print secrets. Keep persisted messages to short,
   operational summaries.
5. Create local data files with least-privilege permissions.
6. Pin third-party GitHub Actions by full commit SHA and keep workflow
   `permissions` at least privilege (`contents: read` unless a job needs more).
7. Verify checksums for any toolchain or binary downloaded in CI.
8. Do not disable or weaken CodeQL, gosec, `go vet`, `golangci-lint`, or tests
   to make a build pass.
9. In workflows, pass values derived from refs, tags, or other user-controlled
   inputs into `run:` scripts through `env:` variables, not inline `${{ }}`
   interpolation. The release smoke step's version check follows this rule.

## Existing controls observed

- External commands use `exec.CommandContext` with separate arguments and a
  default 3s timeout (`health.go`).
- Data access goes through GORM with struct conditions, with no raw SQL.
- Workflows declare `permissions: contents: read` at the top level. Only the
  CD `release` job requests `contents: write`, and `security.yml` requests
  `security-events: write` for SARIF upload.
- All third-party actions are pinned by commit SHA. The windows/arm64
  llvm-mingw download is verified against a pinned SHA-256.
- CI uses `pull_request`, not `pull_request_target`.
- CodeQL (`security-extended`) and gosec run on every push and PR and weekly.
- The linux release builds are statically linked with
  `sqlite_omit_load_extension`, which disables SQLite extension loading.

Not verified during this review: that each pinned SHA matches its version
comment, and the current Code Scanning alert state on GitHub.

## Issues

### SEC-001: Option injection through `--service` into `systemctl`

- **Status:** Closed (2026-09-27)
- **Affected component:** `internal/health.go`, `checkServiceUptime`
- **Risk:** Low. The `--service` value is passed verbatim as an argument to
  `systemctl is-active <name>`. A value that starts with `-` is parsed as a
  `systemctl` option. For example, `--service=--host=user@example` makes
  `systemctl` open an SSH connection to a remote host. This matters when an
  untrusted party can influence the flag, such as a CI parameter or a wrapper
  script.
- **Required remediation:** Validate the name against the systemd unit-name
  character set, rejecting empty values and values that start with `-`. Also
  pass `--` before the unit name (`systemctl is-active -- <name>`). Return
  `StatusFail` with a clear message for invalid names.
- **Validation:** Unit tests (using `fakeToolOptions` in
  `internal/checks_test.go`, available since 2026-09-27) show that
  `--host=x`, `-H`, and an empty string are rejected without executing
  `systemctl`, and that `nginx`, `nginx.service`, and `getty@tty1.service` are
  accepted. The gosec G204 finding for the command wrapper
  (`CheckOptions.command`) is reviewed and documented.
- **Resolution:** Merged in `753252e`; CI run #48 (36367840814), Docker #14, and Security #53 are green on `753252e` (Ubuntu, macOS, Windows). `validUnitName` (in `internal/health.go`)
  accepts only systemd unit-name characters (`A-Za-z0-9:_.\@-`), no leading
  `-`, and at most 255 bytes. Invalid names return `FAIL` ("invalid service
  name …") before `systemctl` is looked up or run. Valid names are passed as
  `systemctl is-active -- <name>`. Validation evidence:
  `TestValidUnitName`, `TestCheckServiceUptimeRejectsInvalidNames` (no command
  runs), and `TestCheckServiceUptimeWithService` (argv includes `--`). gosec
  v2.29.0 G204 on `CheckOptions.command` was reviewed: tool names are
  constants, and the only user input is validated (see `notes.md`, gosec
  baseline).

### SEC-002: Container image runs as root and base images are not digest-pinned

- **Status:** In Progress (implemented 2026-09-27; awaiting the Docker workflow
  with the P2-4 workflow patch applied)
- **Affected component:** `Dockerfile`, `.github/workflows/docker.yml`
- **Risk:** Medium. The runtime stage has no `USER` directive, so `salus` runs
  as root. Combined with the README's guidance to mount the Docker socket, a
  compromised or misused container has root-equivalent control of the host.
  `golang:1.26-alpine` and `alpine:3.22` are referenced by mutable tags, so
  builds are not reproducible and can silently pick up changed images.
  (A `docker.yml` comment that wrongly assumed a non-root user was corrected
  on 2026-09-27 to point at this item.)
- **Required remediation:** Add a dedicated non-root user and group in the
  runtime stage, give that user ownership of `/app/data`, and set `USER`. Pin
  both base images by `@sha256:` digest, with tags kept as comments, and keep
  them current with automated updates (see SEC-005).
- **Validation:** `docker run --rm --entrypoint id <image>` reports a non-zero
  UID. The `docker.yml` smoke test writes to and reads from the `/app/data`
  volume as that user. Both `FROM` lines contain digests.
- **Resolution:** Pending CI.
  - The runtime stage creates `salus` (UID/GID 10001), gives it `/app/data`
    (mode 0700, before `VOLUME`, so new named volumes inherit the owner), and
    sets `USER 10001:10001`.
  - Both stages are pinned by index digest, and Dependabot's `docker`
    ecosystem keeps them current: `golang:1.26-alpine3.24@sha256:8ac98ca5…`
    and `alpine:3.24@sha256:294b683c…`, from Docker Hub on 2026-09-27. The
    runtime moved from Alpine 3.22 to 3.24 to match the builder's musl.
  - The README no longer suggests mounting the Docker socket (SEC-007) and
    documents the one-time `chown` for 1.0.x volumes.
  - Validation is added to `docker.yml` in the P2-4 patch: a
    `Verify non-root user` step (`id -u` must not be 0), and the existing
    volume smoke test now runs as UID 10001.
  - `hadolint` v2.15.1 reports only DL3018 (unpinned `apk` package versions),
    which the Dockerfile already had. The runtime stage no longer installs any
    packages (`sqlite-libs` and `ca-certificates` were unused), which also
    shrinks its attack surface.

### SEC-003: No `.dockerignore`; full working tree is sent to the build context

- **Status:** In Progress (implemented 2026-09-27; awaiting validation)
- **Affected component:** `Dockerfile` (`COPY . .`), repository root
- **Risk:** Low. Without a `.dockerignore`, the entire directory is sent to the
  builder, including `.git/`, `.idea/`, any local `.env`, and any `*.db` files
  in the tree. The final image contains only the binary, but these files reach
  the builder stage, its layer cache, and any remote builder.
- **Required remediation:** Add a `.dockerignore` that excludes at least
  `.git`, `.github`, `.idea`, `.devcontainer`, `.junie`, `.env`, `*.db`,
  `dist/`, coverage outputs, and `intel/`.
- **Validation:** A local build shows a reduced context transfer. The
  `docker.yml` build still succeeds. A test file named `.env` in the tree is
  absent from the builder stage (checked with a temporary `RUN ls -a`
  locally, not committed).
- **Resolution:** Pending validation. `.dockerignore` excludes `.git`,
  `.github`, `.idea`, `.vscode`, `.devcontainer`, `.junie`, `intel`, `.env*`,
  SQLite files (`*.db`, `-journal`, `-wal`, `-shm`), `dist`, coverage and test
  outputs, and local `salus` binaries. A Docker daemon was not available in
  the analysis environment, so the local context check above is still needed.
  The CI build covers the "build still succeeds" part.

### SEC-004: SQLite database created with default permissions in the working directory

- **Status:** Closed (2026-09-27)
- **Affected component:** `internal/database-path.go`, `internal/database.go`
  (`NewDatabase`), `internal/health.go` (`checkMisconfiguration`)
- **Risk:** Low. By default `salus.db` is created in whatever directory Salus
  runs from, with the process umask (commonly `0644`, so world-readable). It
  stores host metadata and the first line of external tool errors, which can
  include cluster endpoints and host names. The `misconfig` check inspects
  permissions only when `SALUS_DB_PATH` is set, and flags only group/other
  write access, not read access. Persisted messages are not length-bounded.
  On Windows the POSIX mode check is skipped (since 2026-09-27, P1-5), because
  Go reports `0666` for every writable file there and the check only produced
  false positives. Windows ACLs are not inspected. This is not a regression:
  the Windows check never produced a meaningful signal.
- **Required remediation:** Create the database file with mode `0600` before
  opening it with GORM, and its parent directory with `0700` when Salus
  creates it. Apply the `misconfig` permission check to the effective
  database path, including the default. Bound the length of persisted
  `Message` values. The default location moves to a per-user data directory
  (Q-002, decided 2026-09-27; implemented under `plan.md` P1-10).
- **Validation:** A unit test on Unix asserts that a newly created database
  has mode `0600`. A `misconfig` test with a `0644` database file returns
  `WARN`. A test asserts truncation of messages longer than the bound.
- **Resolution:** Merged in `753252e`; CI run #48 (36367840814), Docker #14, and Security #53 are green on `753252e` (Ubuntu, macOS, Windows), and the Docker smoke test
  passes with the new owner-only database file.
  - New database files are created with `O_EXCL` and mode `0600`, and missing
    parent directories with `0700`. Existing files are never modified, so
    read-only databases still open.
  - The default path is now per-user (P1-10).
  - `misconfig` checks the effective file (`SALUS_DB_PATH` or the default,
    with go-sqlite3 `?` parameters stripped) and warns on any group/other bit
    (`0o077`), suggesting `chmod 600`.
  - Stored messages are capped at 1024 bytes on a UTF-8 boundary.
  - Evidence: `TestNewDatabaseCreatesOwnerOnlyFileAndDirectories`,
    `TestNewDatabaseKeepsExistingFileMode`,
    `TestNewDatabaseOpensReadOnlyFile`,
    `TestNewDatabaseStripsConnectionParameters`,
    `TestCheckMisconfigurationDatabasePermissions` (0600 pass, 0640 and 0666
    warn), `TestCheckMisconfigurationChecksDefaultDatabasePath`,
    `TestRecordScanBoundsStoredMessages`, and `TestTruncateMessage`. The
    permission tests were also run as an unprivileged user (`nobody`). Windows
    remains out of scope for mode bits (see above).

### SEC-005: No dependency vulnerability scanning or update automation

- **Status:** In Progress (implemented 2026-09-27 as a workflow patch; awaiting
  merge and the validation below)
- **Affected component:** `.github/workflows/`, `.github/` (no `dependabot.yml`)
- **Risk:** Low. Nothing runs `govulncheck` against the module graph. SHA-pinned
  actions, Go modules, and Docker base images have no automated update path,
  so pins go stale and known-vulnerable versions can persist. gosec runs with
  `-no-fail`, so its findings never block a merge. This is an accepted design
  decision (Q-009, 2026-09-27), which makes triage in Code Scanning the only
  control for gosec findings.
- **Required remediation:** Add a `govulncheck ./...` step to CI or
  `security.yml` that fails on reachable vulnerabilities. Add
  `.github/dependabot.yml` for the `gomod`, `github-actions`, and `docker`
  ecosystems. Keep gosec non-blocking per Q-009, and record that policy and
  the triage expectation in `maint.md`.
- **Validation:** The workflow run shows `govulncheck` executing and failing
  on a known-vulnerable test fixture or version (checked once on a branch).
  Dependabot opens update PRs. The non-blocking gosec policy is documented in
  `maint.md`.
- **Resolution:** Pending merge of `salus-p24-workflows.patch`.
  - `security.yml` gains a `govulncheck` job with job-level `contents: read`.
    It runs on every push and PR and weekly, and executes
    `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`, which exits
    non-zero on reachable vulnerabilities. That version is pinned manually
    because Dependabot does not see it.
  - `.github/dependabot.yml` covers `gomod`, `github-actions`, and `docker`
    weekly.
  - The gosec policy is already recorded in `maint.md` section 7.
  - `govulncheck` could not be run in the analysis environment (the
    vulnerability database host is blocked), so the first CI run is the
    first result.

### SEC-006: Release artifacts are not signed and have no provenance

- **Status:** Open
- **Affected component:** `.github/workflows/cd.yml` (`release` job)
- **Risk:** Low. `checksums.txt` is produced in the same job and published
  next to the artifacts. It detects corruption but not tampering, because an
  attacker who can replace an artifact can also replace the checksum file.
  Users have no way to verify that a binary was built by this repository's
  workflow.
- **Required remediation:** Generate build provenance attestations for the
  release archives, for example with GitHub's artifact attestation action
  pinned by SHA and `id-token: write` / `attestations: write` scoped to the
  release job only. Alternatively, keylessly sign `checksums.txt`. Document
  the verification command in `README.md`.
- **Validation:** Verification of one release artifact succeeds with the
  documented command, and verification of a modified artifact fails.
- **Resolution:** None yet.

### SEC-007: README recommends mounting the Docker socket into the container

- **Status:** In Progress (README updated 2026-09-27; awaiting the CI assertion)
- **Affected component:** `README.md` ("Docker" section), `Dockerfile`
- **Risk:** Low (documentation). Mounting `/var/run/docker.sock` gives the
  container root-equivalent control of the host (compounded by SEC-002). The
  runtime image also contains no `docker` or `kubectl` binaries, so the
  Docker and Kubernetes checks report `WARN` ("CLI not found in PATH") even
  when the socket or a kubeconfig is mounted. The guidance adds risk without
  delivering the function.
- **Required remediation:** Update the README to state that in-container
  Docker and Kubernetes checks are unsupported by the current image, and to
  recommend running the binary on the host for those checks. If container
  support is wanted, document the socket risk explicitly and mount kubeconfig
  read-only, as a deliberate and reviewed decision that adds the CLIs to the
  image.
- **Validation:** README review. `docker run` of the image, with and without
  the socket mount, matches the documented behavior.
- **Resolution:** Pending CI. The README Docker section now says the
  `docker-status` and `kubernetes-status` checks are unsupported in the image
  (they report `WARN`), recommends running the binary on the host, and warns
  against mounting the Docker socket. The P2-4 patch adds a `docker.yml` step
  that asserts both checks report `WARN` with "CLI not found in PATH" inside
  the image. No socket is involved: the result does not depend on a socket,
  because the CLIs are absent.

### SEC-008: Release binaries built with an outdated Go patch release

- **Status:** In Progress (fixed in the working tree 2026-09-27; awaiting CI and
  a new release)
- **Affected component:** `go.mod` (`go` directive); `.github/workflows/cd.yml`
  and `ci.yml` (`setup-go` with `go-version-file: go.mod`); the v1.0.0 release
  archives
- **Risk:** Low to Medium; the exact exposure is unknown until `govulncheck`
  runs. `go.mod` declared `go 1.26.0`, and `setup-go` installs exactly that
  version, so CI tested with, and CD built v1.0.0 with, Go 1.26.0. The CI log
  for run 36367840814 shows `go version go1.26.0`. Standard-library security
  fixes from Go 1.26.1 through 1.26.8 are missing from those binaries. Salus is
  a local CLI with no network listener, which limits reachability. The Docker
  image was not affected: its builder already used Go 1.26.8.
- **Required remediation:** Set the `go` directive to the latest 1.26 patch
  release (1.26.8), and keep it current (see `maint.md` section 5). Run
  `govulncheck` in CI (SEC-005). Publish a new release built with the patched
  toolchain.
- **Validation:** The CI and CD logs show `go version go1.26.8`. The
  `govulncheck` job passes. `go version -m` on a new release binary reports
  `go1.26.8`.
- **Resolution:** Pending. `go.mod` is now `go 1.26.8`. Locally, tests pass
  and `go version -m` on the built binary reports `go1.26.8`. It still needs a
  CI run and a new release.
