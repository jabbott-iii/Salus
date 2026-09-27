# Security Requirements and Issue Tracking

Security requirements, identified issues, remediation items, and their status.
Rules for this file are in `AGENTS.md` ("Security Issue Tracking"): never
delete items, mark `Closed` only after remediation and validation, and never
regress a documented remediation.

Last reviewed: 2026-09-27 (against commit `460a24b` plus uncommitted Phase 0
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

- **Status:** Open
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
- **Validation:** Unit tests (with an injectable command runner) show that
  `--host=x`, `-H`, and an empty string are rejected without executing
  `systemctl`, and that `nginx`, `nginx.service`, and `getty@tty1.service` are
  accepted. The gosec G204 finding for this call site is reviewed and
  documented.
- **Resolution:** None yet.

### SEC-002: Container image runs as root and base images are not digest-pinned

- **Status:** Open
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
- **Resolution:** None yet.

### SEC-003: No `.dockerignore`; full working tree is sent to the build context

- **Status:** Open
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
- **Resolution:** None yet.

### SEC-004: SQLite database created with default permissions in the working directory

- **Status:** Open
- **Affected component:** `database_path.go`, `internal/database.go`
  (`NewDatabase`), `internal/health.go` (`checkMisconfiguration`)
- **Risk:** Low. By default `salus.db` is created in whatever directory Salus
  runs from, with the process umask (commonly `0644`, so world-readable). It
  stores host metadata and the first line of external tool errors, which can
  include cluster endpoints and host names. The `misconfig` check inspects
  permissions only when `SALUS_DB_PATH` is set, and flags only group/other
  write access, not read access. Persisted messages are not length-bounded.
- **Required remediation:** Create the database file with mode `0600` before
  opening it with GORM, and its parent directory with `0700` when Salus
  creates it. Apply the `misconfig` permission check to the effective
  database path, including the default. Bound the length of persisted
  `Message` values. The default location moves to a per-user data directory
  (Q-002, decided 2026-09-27; implemented under `plan.md` P1-10).
- **Validation:** A unit test on Unix asserts that a newly created database
  has mode `0600`. A `misconfig` test with a `0644` database file returns
  `WARN`. A test asserts truncation of messages longer than the bound.
- **Resolution:** None yet.

### SEC-005: No dependency vulnerability scanning or update automation

- **Status:** Open
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
- **Resolution:** None yet.

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

- **Status:** Open
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
- **Resolution:** None yet.
