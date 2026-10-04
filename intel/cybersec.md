# Security Requirements and Issue Tracking

Security requirements, identified issues, remediation items, and their status.
Rules for this file are in `AGENTS.md` ("Security Issue Tracking"): never
delete items, mark `Closed` only after remediation and validation, and never
regress a documented remediation.

Last reviewed: 2026-10-04 (against `8856673`, which carries M5 and M6;
v1.0.2 is at `08b2faa`). The M6 review of new inputs and outputs found no new
issue; its controls are listed under "Existing controls observed". SEC-009 is
Closed; SEC-003 and SEC-005 still wait on maintainer checks.

## Threat model summary

- **Deployment:** Local CLI run by an operator, a cron job, a CI step, or a
  monitoring agent (Nagios-style plugin, Prometheus textfile collector); also
  shipped as a container image. No network listener and no HTTP API.
- **Trust boundaries:**
  - Command-line flags: `--service`, `--kube-context`, `--kube-namespace`,
    `--disk-path`, `--cert`, `--output`, `--only`, the thresholds, and the
    ages of `jobs prune --older-than`, `jobs stats --since`, and
    `check run --retain`.
  - Environment variables: `SALUS_DB_PATH`, `SALUS_SSHD_CONFIG`,
    `KUBECONFIG`, `DOCKER_HOST`, `DOCKER_TLS_VERIFY`, `PATH`, and `HOME`.
  - Output of external tools (`docker`, `kubectl`, `systemctl`,
    `timedatectl`), which a hostile daemon or API server can control
    (SEC-009).
  - Files read on the user's behalf: `--cert` files (which may also hold a
    private key) and `sshd_config` with its includes.
  - The SQLite file on disk, and the `--output` report file.
  - The CI/CD supply chain: actions, base images, Go modules, and release
    artifacts.
- **Data handled:** Host health metadata:
  - mount paths, service names, and the Docker server version;
  - container, node, pod, and systemd unit names;
  - certificate subject names and validity dates;
  - kubeconfig, socket, and `PATH` directory paths;
  - the first line of external tool errors, which can include cluster
    endpoints or host names;
  - timestamps.

  Salus reads only the permission bits of kubeconfig files, never their
  contents. A `--cert` file may contain a private key; Salus parses only its
  certificate blocks and prints only certificate metadata. No credentials are
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
10. Jobs that hold `id-token: write` run only first-party actions
    (`actions/*`). Any step in such a job can mint signing credentials for the
    workflow, so a third-party action there could forge provenance (SEC-006).
11. Treat external tool output as untrusted. Parse it against the expected
    format, and neutralize control characters before a message is printed or
    stored (`sanitizeMessage`, SEC-009).
12. Read files on the user's behalf read-only, with a size bound where the
    format allows one, and never copy their contents into messages or
    reports; report metadata only.
13. Escape every value placed in a structured report format for that format
    (Prometheus label escaping, `encoding/xml` for JUnit, quoting and
    separator replacement for Nagios), so a target or message cannot forge
    another line, label, or element.

## Existing controls observed

- External commands use `exec.CommandContext` with separate arguments and a
  default 3s timeout (`health.go`, configurable with `--timeout`).
- User values that reach external tools are validated first (requirement 2).
  - `--service` is passed after `--` (SEC-001).
  - `--kube-context` must pass `validKubeContext`: no leading `-`, no control
    characters, valid UTF-8 of at most 253 bytes. It is passed as the single
    argument `--context=<name>`.

  - `--kube-namespace` must pass `validNamespace` (an RFC 1123 label, at most
    63 bytes) and is passed as the single argument `--namespace=<name>` (M6).

  Invalid values fail the check without running the tool.
- M6 inputs and outputs (2026-10-03):
  - `RunChecks` sanitizes targets as well as messages, and stored targets are
    sanitized again when `jobs show`, `jobs diff`, and `jobs stats` print
    them (`resultOutcome`).
  - `--cert` files are opened read-only and read up to 1 MiB
    (`maxCertFileSize`); non-certificate PEM blocks, such as private keys, are
    skipped, and messages carry only the path, subject name, and dates
    (tested with a key in the same file).
  - The sshd rules read `sshd_config` and its includes read-only, limit
    include depth to 16 (as OpenSSH does, which also stops include cycles),
    and report nothing when any needed file or directory cannot be read.
  - `docker-tcp-insecure` reports only the host and port from `DOCKER_HOST`,
    never user information or a path (tested with credentials in the URL).
  - Report formats escape their values (requirement 13): Prometheus label
    values escape `\`, `"`, and newlines and never include messages; JUnit
    uses `encoding/xml`; Nagios labels double `'` and replace `=` and `|`, and
    `|` in messages is replaced. Golden tests cover each case.
  - `--output` writes through `os.CreateTemp` (random name, `O_EXCL`, `0600`)
    in the target directory and renames over the target, so it never writes
    through a symlink or leaves a partial file; a target that is a symbolic
    link or any other non-regular file is rejected before any check runs. A
    new report file is `0600` (requirement 5); an existing file keeps its
    permission bits and, where permitted, its group.
  - `--cert` files, sshd_config, and its includes are opened only if they are
    regular files, so a FIFO cannot block a scheduled run.
  - New SQL (`jobs stats`) uses `?` placeholders and a subquery, like
    `jobs prune`.
- P3-7 platform calls (2026-10-04):
  - Windows: `kernel32.dll` is loaded with `syscall.NewLazyDLL`, which the
    standard library loads from System32 only because `syscall` registers it
    as a system DLL, so no DLL preloading is possible. The only user value
    passed is the `--disk-path`, as a UTF-16 string to
    `GetDiskFreeSpaceExW`; a path with a NUL byte is rejected by
    `UTF16PtrFromString`. `unsafe` is limited to passing pointers to local
    variables in `LazyProc.Call` argument lists.
  - macOS: only fixed sysctl names are read; returned sizes are checked
    before decoding.
- `jobs prune` deletes through parameterized GORM queries (requirement 3).
- The `misconfig` check reports insecure host settings: group- or
  other-accessible database and kubeconfig files, a Docker socket writable by
  all users, `PATH` directories writable by all users, and (M6) a Docker TCP
  endpoint without TLS verification and an SSH server that permits root login
  or password authentication.
- Data access goes through GORM with struct conditions or string conditions
  with `?` placeholders (`jobs prune` uses `julianday(started_at) <
  julianday(?)`). No SQL is built from input.
- Workflows declare `permissions: contents: read` at the top level. Only the
  CD `release` job requests `contents: write`, and `security.yml` requests
  `security-events: write` for SARIF upload.
- All third-party actions are pinned by commit SHA. The windows/arm64
  llvm-mingw download is verified against a pinned SHA-256.
- CI uses `pull_request`, not `pull_request_target`.
- CodeQL (`security-extended`) and gosec run on every push and PR and weekly.
- The linux release builds are statically linked with
  `sqlite_omit_load_extension`, which disables SQLite extension loading.
- Since v1.0.1 (`b66694a`):
  - `govulncheck` runs in `security.yml`, and Dependabot covers `gomod`,
    `github-actions`, and `docker` (SEC-005).
  - The container image runs as UID 10001 on digest-pinned bases (SEC-002).
  - Releases are built with the patched Go toolchain (SEC-008).
- GitHub marks v1.0.1 as an immutable release with a release attestation
  (see SEC-006).
- Pinned action SHAs match their version comments (checked 2026-09-27 by
  resolving each tag through the GitHub API, dereferencing annotated tags).
  This covers every pin on `main`, the pins in Dependabot PRs #13 and #18 to
  #21, and the new pins for `actions/attest` v4.2.2 (`1e69f48a…`) and
  `softprops/action-gh-release` v3.0.3 (`efb35369…`). Both new pins are
  signed commits.
- Branch rulesets require a pull request with a code-owner approval, merge
  commits only, and a passing CodeQL code scanning check. Branch and tag
  deletion and force pushes are blocked. The repository admin role can
  bypass them.
- Since v1.0.2 (`08b2faa`), the CD `package` job attests build provenance for
  every release archive. It is the only job with
  `id-token: write` and `attestations: write`, and it runs only first-party
  actions. The third-party release action runs in the separate `release` job,
  which has `contents: write` but cannot sign.

Not verified during this review: the current Code Scanning alert state on
GitHub, because the available token cannot read it.

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

- **Status:** Closed (2026-09-27)
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
- **Resolution:** Merged in `b66694a` (v1.0.1). Validated by Docker run #15
  (36378207180, job 108788270272) on `b66694a`:
  - The build log resolves both `FROM` lines by digest
    (`golang:1.26-alpine3.24@sha256:8ac98ca5…` and
    `alpine:3.24@sha256:294b683c…`).
  - `Verify non-root user` printed `image user id: 10001`.
  - The volume smoke test ran as that user: `check run --only misconfig`
    reported `[PASS]`, and `jobs show 1` read the job back from the
    `salus-smoke` volume.
  - Dependabot's `docker` update job ran on the same push (run 36378211944)
    and opened no PR, because both digests were current.

  Implementation details:
  - The runtime stage creates `salus` (UID/GID 10001), gives it `/app/data`
    (mode 0700, before `VOLUME`, so new named volumes inherit the owner), and
    sets `USER 10001:10001`.
  - Both stages are pinned by index digest, and Dependabot's `docker`
    ecosystem keeps them current: `golang:1.26-alpine3.24@sha256:8ac98ca5…`
    and `alpine:3.24@sha256:294b683c…`, from Docker Hub on 2026-09-27. The
    runtime moved from Alpine 3.22 to 3.24 to match the builder's musl.
  - The README no longer suggests mounting the Docker socket (SEC-007) and
    documents the one-time `chown` for volumes created by 1.0.0.
  - Validation is in `docker.yml` (P2-4 patch, merged in `b66694a`): a
    `Verify non-root user` step (`id -u` must equal 10001), and the existing
    volume smoke test now runs as UID 10001.
  - `hadolint` v2.15.1 reports only DL3018 (unpinned `apk` package versions),
    which the Dockerfile already had. The runtime stage no longer installs any
    packages (`sqlite-libs` and `ca-certificates` were unused), which also
    shrinks its attack surface.

### SEC-003: No `.dockerignore`; full working tree is sent to the build context

- **Status:** In Progress (merged in v1.0.1; awaiting the local `.env` check)
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
- **Resolution:** Pending the local check. `.dockerignore` excludes `.git`,
  `.github`, `.idea`, `.vscode`, `.devcontainer`, `.junie`, `intel`, `.env*`,
  SQLite files (`*.db`, `-journal`, `-wal`, `-shm`), `dist`, coverage and test
  outputs, and local `salus` binaries. Merged in `b66694a` (v1.0.1).
  - Build still succeeds: Docker run #15 (36378207180) on `b66694a` built the
    image. It sent a 358 B `.dockerignore` and a 159 kB build context.
  - Supporting evidence (2026-09-27), which is not the documented check: the
    `.dockerignore` was evaluated with `github.com/moby/patternmatcher`
    v0.6.1, the parser and matcher BuildKit uses. The input was a copy of the
    `b66694a` tree plus decoys: `.env`, `.env.local`, `internal/.env`,
    `salus.db` with `-wal` and `-shm` files, `internal/test.db-journal`,
    `coverage.out`, `internal/c.test`, `salus`, `salus-ci.exe`, `dist/…`, and
    `.vscode/…`. Only the 36 source, build, and documentation files would be sent.
    No decoy, and nothing under `.git`, `.github`, `.idea`, or `intel`, was
    included.
  - Still needed: the local check, on a machine with Docker, without editing
    the Dockerfile. Run these from the repository root:

    ```bash
    printf 'SEC003_CHECK=1\n' > .env
    docker build --no-cache --progress=plain --target builder -t salus-ctx-check . 2>&1 | grep 'transferring context'
    docker run --rm --entrypoint ls salus-ctx-check -a /src   # expect no .env
    docker image rm salus-ctx-check && rm .env
    ```

    Neither the analysis environment nor the linked computer's workspace had
    a Docker daemon available.

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

- **Status:** In Progress (merged in v1.0.1; awaiting the one-time failing
  `govulncheck` run)
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
- **Resolution:** Merged in `b66694a` (v1.0.1). Two of the three validation
  points are met:
  - `govulncheck` executes: Security run #54 (36378207181, job 108788270515)
    on `b66694a` ran it with Go 1.26.8 and printed `No vulnerabilities found.`
  - Dependabot opens update PRs: the first `github-actions` run opened #13
    (`actions/setup-go` 5.6.0 → 7.0.0), #14 (`actions/upload-artifact`
    4.6.2 → 7.0.1), #15 (`github/codeql-action/upload-sarif` 3.38.1 →
    4.38.2), #16 (`actions/download-artifact` 4.3.0 → 8.0.1), and #17
    (`github/codeql-action/autobuild` 3.38.1 → 4.38.2). That is five PRs,
    Dependabot's default open-PR limit. The `gomod` and `docker` runs
    succeeded and opened nothing.
  - The gosec policy is in `maint.md` section 7.

  Still needed: the run must also show `govulncheck` failing once. This is
  checked on a throwaway branch that is never merged. On that branch, require
  `github.com/dgrijalva/jwt-go@v3.2.0+incompatible` and add a `package main`
  file whose `init` calls `jwt.MapClaims{}.VerifyAudience("salus", false)`.
  That symbol is affected by GO-2020-0017 (CVE-2020-26160), with no fixed v3
  version, per `golang/vulndb` `data/reports/GO-2020-0017.yaml`. Pushing the
  branch triggers `security.yml`, which runs on every branch, and the
  `govulncheck` job should fail with exit code 3. The fixture compiles against
  v3.2.0; this was checked locally on 2026-09-27. Delete the branch
  afterwards.

  Supporting evidence (2026-09-27), which is not the documented check:
  - In a scratch copy of the tree with that fixture, `govulncheck` v1.8.0 (the
    version `security.yml` pins) exited 3 and reported GO-2020-0017 in
    `github.com/dgrijalva/jwt-go@v3.2.0+incompatible`.
  - For the unmodified tree it reported `No vulnerabilities found.`
  - The branch rulesets block branch deletion, but the repository admin role
    can bypass them, so the maintainer can delete the throwaway branch.
  - The agent session could not push the branch itself, because `AGENTS.md`
    forbids it from creating commits.

  Operational note: #15 and #17 each bump only one `github/codeql-action`
  sub-action. #17 fails the CodeQL job (`Loaded a configuration file for
  version '3.38.1', but running version '4.38.2'`). #15 passes, but it leaves
  mixed versions. Neither should be merged alone; see `plan.md` P2-8.
  Resolved 2026-09-27: after the grouping in `78db94e`, Dependabot closed #14
  to #17 and opened #18, which moves all four `github/codeql-action`
  sub-actions to v4.38.2, and #19, which moves both artifact actions. The
  Security workflow passes on #18.

  Implementation details:
  - `security.yml` gains a `govulncheck` job with job-level `contents: read`.
    It runs on every push and PR and weekly, and executes
    `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`, which exits
    non-zero on reachable vulnerabilities. That version is pinned manually
    because Dependabot does not see it.
  - `.github/dependabot.yml` covers `gomod`, `github-actions`, and `docker`
    weekly.
  - The gosec policy is already recorded in `maint.md` section 7.
  - `govulncheck` could not be run in the analysis environment, because the
    vulnerability database host is blocked. The CI runs are the only
    `govulncheck` results.

### SEC-006: Release artifacts are not signed and have no provenance

- **Status:** Closed (2026-09-28)
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
- **Resolution:** Closed 2026-09-28, after the validation on v1.0.2 described at
  the end of this item.
  - Observed 2026-09-27: GitHub shows v1.0.1 as an **Immutable** release with
    a "Release attestation (json)" asset. Its assets cannot be replaced after
    publication. The release attestation binds the asset digests to the
    release, so it can be checked with
    `gh release verify-asset v1.0.1 <file>` (GitHub CLI; not run here).
  - This addresses tampering after publication. It does not show that the
    workflow built the binaries: the release attestation covers release
    membership, not build provenance. The required remediation above still
    stands.
  - Whether the immutable-release attestation is an acceptable substitute is
    a maintainer decision; it is not assumed here.
  - **Decision (2026-09-27, Q-010):** add build provenance. The release
    attestation alone is not accepted as a substitute.
  - **Implementation (merged in `08b2faa`, released in v1.0.2):** `cd.yml`
    splits the old `release` job in two.
    - `package` ("Package and attest") downloads the binaries, packages them,
      writes `checksums.txt`, and runs `actions/attest` v4.2.2 (pinned at
      `1e69f48a…`) with `subject-checksums: dist/checksums.txt`, so all six
      archives are subjects. It then uploads the archives as the
      `release-archives` artifact.
    - Only `package` has `id-token: write` and `attestations: write` (plus
      `contents: read`), and it runs only first-party actions (security
      requirement 10). `artifact-metadata: write` is omitted: in the v4.2.2
      source, storage records are created only with `push-to-registry` and only
      for organization-owned repositories.
    - `release` ("Publish release") downloads `release-archives` and runs
      `softprops/action-gh-release` v3.0.3 with `contents: write` on tag runs.
      A compromised release action could therefore change what is uploaded,
      but it could not sign provenance for it, so verification would fail.
      softprops v3.0.3 runs on Node 24 with no input changes, and v3.0.2 fixed
      uploads of small assets such as `checksums.txt`.
    - Both jobs also run on manual runs, where only the `Create GitHub Release`
      step is skipped. Attestation therefore precedes publication, and a
      failed attestation publishes nothing.
    - The new artifact steps use `upload-artifact` v7.0.1 and
      `download-artifact` v8.0.1, the versions Dependabot PR #19 moves the
      existing steps to. download-artifact v8 fails on a digest mismatch by
      default.
    - The README documents the strict check:
      `gh attestation verify <archive> --repo jabbott-iii/Salus --signer-workflow jabbott-iii/Salus/.github/workflows/cd.yml --source-ref refs/tags/<tag>`.
      It requires GitHub CLI 2.97 or newer: `--source-ref` needs 2.68, and
      2.97.0 fixed GHSA-mm27-mwq9-fr5g, a signer-matching bypass.
  - **Independent review findings (2026-09-27), both fixed before commit:**
    - With `--repo` alone, gh accepts an attestation signed by any workflow in
      the repository, from any ref (`policy.go` in gh v2.101.0). Manual CD
      runs from any branch also create public attestations for archives with
      the same names. The `--signer-workflow` and `--source-ref` flags exclude
      both.
    - The first implementation gave the single release job both
      `id-token: write` and `contents: write` next to a third-party action.
      That would have let a compromised action upload a malicious archive and
      sign matching provenance. Hence the job split.
  - **Local validation:**
    - actionlint 1.7.12 reports no findings, including after merging the open
      Dependabot PRs into a scratch copy.
    - The packaging and checksum commands were run on dummy binaries. The
      resulting `checksums.txt` parses, with the logic of the v4.2.2
      `src/subject.ts`, into exactly the six archives, without
      `checksums.txt` itself.
    - The multi-path upload and the by-name download place the files directly
      in `dist/`, per the upload-artifact v7.0.1 README (least common ancestor
      as the root) and the download-artifact v8.0.1 README.
  - **Validation (2026-09-28, v1.0.2):**
    - CD #3 (36397627324, tag `v1.0.2` at `08b2faa`) ran the `package` job,
      which logged `Attestation created for 6 subjects` (Rekor log index
      2981647855; `https://github.com/jabbott-iii/Salus/attestations/50705519`).
      The `release` job downloaded `release-archives` and published the
      immutable release.
    - The SLSA statement's six subjects match the six release asset digests
      exactly. Its certificate (issuer `sigstore-intermediate`) names
      `https://github.com/jabbott-iii/Salus/.github/workflows/cd.yml@refs/tags/v1.0.2`
      as the signer, with source ref `refs/tags/v1.0.2`.
    - The documented command, run with GitHub CLI 2.101.0 (built from source,
      isolated configuration) on `salus_linux_amd64.tar.gz` (which matched
      `checksums.txt`), exited 0. It reported SLSA provenance v1, source
      commit `08b2faae…`, a GitHub-hosted runner, and one verified
      transparency-log timestamp.
    - A copy with one byte appended failed (exit 1; the API has no attestation
      for its digest).
    - The real archive with `--source-ref refs/heads/main` failed (exit 1:
      `expected SourceRepositoryRef to be refs/heads/main, got refs/tags/v1.0.2`).
    - `gh attestation verify` needs an authenticated GitHub CLI even for this
      public repository, so the README now says so.

    The v1.0.0 and v1.0.1 archives stay without build provenance.

### SEC-007: README recommends mounting the Docker socket into the container

- **Status:** Closed (2026-09-27)
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
- **Resolution:** Merged in `b66694a` (v1.0.1). The README Docker section
  says the `docker-status` and `kubernetes-status` checks are unsupported in
  the image (they report `WARN`), recommends running the binary on the host,
  and warns against mounting the Docker socket. It was reviewed against the
  code on 2026-09-27.
  - **Without the socket:** Docker run #15 (36378207180) on `b66694a` passed
    `Smoke test unsupported in-container checks`. Exit code 1, with `WARN` and
    `docker CLI not found in PATH` / `kubectl CLI not found in PATH`.
  - **With the socket:** not run with the image, because no Docker daemon was
    available, and the README advises against mounting the socket. It was
    validated in two ways instead:
    - `checkDockerStatus` and `checkKubernetesStatus` return `WARN` from
      `hasTool` before any socket, `DOCKER_HOST`, or kubeconfig is used,
      which is covered by `internal/checks_test.go`.
    - The released v1.0.1 `salus_linux_amd64` binary (SHA-256 verified
      against `checksums.txt`) was run with a listening Unix socket at
      `/var/run/docker.sock`, with `DOCKER_HOST` set, with a kubeconfig in
      `$HOME/.kube`, and with neither CLI on `PATH`. It gave the same two
      `WARN` results and exit code 1.

### SEC-008: Release binaries built with an outdated Go patch release

- **Status:** Closed (2026-09-27)
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
- **Resolution:** Fixed in `b66694a` (`go.mod` `go 1.26.8`) and released as
  v1.0.1 on 2026-09-27:
  - CI #49 (36378207188) on `b66694a`: `Setup go version spec 1.26.8`,
    `go version go1.26.8 linux/amd64`, tests and golangci-lint pass.
  - CD #2 (36378369985, tag `v1.0.1`), `Build linux/amd64` job
    108788751088: `go version go1.26.8`. All six build jobs and
    `Package and Release` succeeded.
  - Security #54 (36378207181): the `govulncheck` job used Go 1.26.8 and
    found no vulnerabilities.
  - Release binary: `salus_linux_amd64.tar.gz` from the v1.0.1 release
    matched `checksums.txt`. The `checksums.txt` SHA-256 `d0872c3c…` equals
    the digest GitHub shows for it. `go version -m` on the binary reports
    `go1.26.8`, `mod github.com/jabbott-iii/Salus v1.0.1`, and tags
    `sqlite_omit_load_extension,osusergo,netgo`. `--version` prints
    `salus version v1.0.1`.
  - Users on v1.0.0 archives should upgrade to v1.0.1 or later. v1.0.1 also
    carries behavior changes (see `notes.md`, "Upgrade impact").
  - No regression from Dependabot PR #13 (2026-09-27): it moves
    `actions/setup-go` from 5.6.0 to 7.0.0, and v6 changed toolchain
    selection. Its CI logs on all three operating systems still show
    `Setup go version spec 1.26.8` and `go version go1.26.8`.
  - v1.0.2 (2026-09-28): the CD #3 `Build linux/amd64` log shows
    `Setup go version spec 1.26.8` and `go version go1.26.8`.

### SEC-009: External tool output reaches the terminal and the database unfiltered

- **Status:** Closed (2026-10-04)
- **Affected component:** `internal/health.go` (check messages built from
  `docker`, `kubectl`, and `systemctl` output) and `internal/logic-cli.go`
  (`jobs show` text output)
- **Risk:** Low. Check messages embed text from external tools: the first
  line of an error, the Docker server version, and, since P3-2 and P3-3,
  container and node names. A hostile or compromised Docker daemon or
  Kubernetes API server controls that text, and it is reachable when
  `DOCKER_HOST` or a kubeconfig points at it. The text could carry terminal
  escape sequences:
  - In the text report, they could rewrite the screen, for example to hide a
    FAIL line or fake a PASS.
  - They were stored in the database, and `jobs show` replayed them.

  JSON output was only partly protected: `encoding/json` escapes C0 characters
  such as ESC, but emits DEL and C1 characters (such as U+009B, a one-byte
  CSI) unchanged. Found on 2026-09-28 while extending the threat model for M5.
- **Required remediation:** Replace control characters in every check message
  before it is printed or stored. Do the same for stored messages when
  `jobs show` prints them as text.
- **Validation:** Unit tests show that escape sequences from a fake tool are
  replaced in `RunChecks` output, and that stored control characters are
  replaced in `jobs show` text output. CI is green.
- **Resolution:** Implemented 2026-09-28 and committed in `28e66d0`. Both
  validation points are met: the unit tests below pass, and CI is green on
  `28e66d0` (CI #78 on ubuntu, macOS, and Windows; Docker #34; Security #83)
  and on `8856673` (CI #79), which also sanitizes targets (M6).
  - `sanitizeMessage` replaces every Unicode control character (C0, DEL, and
    C1) with `?`.
  - `RunChecks` applies it to every outcome, so the report, `--json`, and the
    database all get the cleaned text.
  - `jobs show` applies it to stored messages, which covers rows written by
    earlier versions. This covers both the text output and `--json`
    (`WriteJobJSON`), because JSON does not escape DEL or C1 characters. An
    independent review found the `--json` gap before commit.
  - Tests: `TestSanitizeMessage`, `TestRunChecksSanitizesToolOutput`, and
    `TestJobsShowSanitizesStoredMessages` (text and `--json`, with ESC, DEL,
    and C1).
