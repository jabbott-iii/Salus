# Engineering Notes and Open Questions

Durable engineering notes and unresolved technical questions. Active work items
live in [`plan.md`](plan.md), security items in [`cybersec.md`](cybersec.md).

Last reviewed: 2026-10-04 (against `03964ff`, which carries M5, M6, and
P3-7; v1.0.2 is at `08b2faa`).

## Engineering notes

### Lineage
- Salus was converted from an earlier project named "Rete" on 2026-09-03/04
  (commits `3a67f29`, `8d90da9`, `3b752f2`). Remnants:
  - the `Dockerfile` comment "Persist sqlite database file (rete.db)";
  - the `Dockerfile` comment "This app is an interactive TUI/CLI". Salus has
    no TUI today;
  - empty `internal/logic-tui.go` and `internal/ui-form.go`. *Removed
    2026-09-28 (P5-2), because Salus is CLI-only per Q-001;*
  - before `7235211`, `NOTICE` listed `charmbracelet/bubbletea` and
    `charmbracelet/lipgloss`, which are not in `go.mod`.

### CI/CD in commit `7235211` was adapted from another project
The workflows reference a different binary ("munus"), which does not match
Salus:
- binary and artifact names are `munus-ci`, `munus_<os>_<arch>`, and image tag
  `munus:<sha>`;
- the DB variable is `MUNUS_DB_PATH`, but Salus reads `SALUS_DB_PATH`;
- the smoke commands are `--version`, `add --title ... --description ...`, and
  `list`. Salus has no root `--version` flag (the Cobra root sets no
  `Version`) and no `add` or top-level `list` command;
- `-ldflags "-X main.version=..."` names a variable that does not exist in
  package `main`. The Go linker silently ignores `-X` for missing symbols, so
  this is a no-op rather than an error.

`7235211` was pushed to `origin/main` together with `460a24b`, so the CI
smoke step and the Docker smoke step are expected to fail on GitHub until the
Phase 0 changes land (GitHub Actions results were not inspected). As of
2026-09-27 the working tree renames everything to Salus, uses `SALUS_DB_PATH`,
smoke-tests real Salus commands, and adds a `main.version` variable so the
`-X` ldflag and `--version` work. See Phase 0 in `plan.md`.

The CGO build strategy in `cd.yml` fits Salus and should be kept when
renaming: native runners per OS, pinned llvm-mingw for windows/arm64, and
static Linux linking with `sqlite_omit_load_extension,osusergo,netgo`.

### Behavior worth knowing
- **Exit code 1 meant two things.** `check run` exited `1` for WARN, and
  `main` also exited `1` for any Cobra error. *Resolved 2026-09-27 (P1-9):*
  operational errors now exit `3`.
- **`os.Exit` inside `check run`, and the DB was never closed.** *Resolved
  2026-09-27 (P1-1, P1-4):* the command returns `*ExitStatusError`, and
  `main.run` closes the database and maps the exit code. The unclosed handle
  was what failed every database test on Windows (`t.TempDir` cleanup could
  not delete the open `salus_test.db`; CI run 36343455341).
- **Job timestamps.** *Resolved 2026-09-27 (P1-2):* `StartedAt` is now the
  time the checks began. Still true: the `running` and `failed` job statuses
  are never visible, because the job is written and completed in one
  transaction and a failed transaction leaves no row.
- **Read outside the transaction.** *Resolved 2026-09-27 (P1-2):*
  `featureByKey` now uses the transaction handle.
- **DB is opened for every command,** including `--help`, `--version`,
  `check list`, and `check run --no-save`, so `salus.db` is created in the current directory
  even when nothing is persisted. *Resolved 2026-09-27 (P1-10):* commands open
  the database lazily, and the default is a per-user path.
- **Upgrade impact of P1-10 and SEC-004 (shipped in v1.0.1).** 1.0.0
  users' `./salus.db` is no longer read by default, so job history looks empty
  until they move the file or set `SALUS_DB_PATH`. Databases created by 1.0.0
  (typically `0644`, including Docker volumes at `/app/data/salus.db`) now
  make `misconfig` WARN, so `check run` exits `1` until the file is
  `chmod 600`. Both are documented in the README ("Upgrading from 1.0.0").
  Printing a stderr hint when `./salus.db` exists was considered and not done,
  to avoid per-run noise. v1.0.1 was cut from `main`, so these behavior changes
  shipped under a patch version, next to the Go 1.26.8 security rebuild
  (SEC-008). Its generated release notes hold only the changelog link. Adding a
  pointer to the README upgrade section is the remaining mitigation, and
  release notes stay editable on an immutable release. *Update 2026-09-28:*
  the v1.0.2 release notes point to the upgrade section and also describe the
  threshold flags and the provenance check. v1.0.1's notes still hold only the
  changelog link.
- **gosec baseline (v2.29.0, run locally 2026-09-27; non-blocking in CI).**
  Four findings, all reviewed:
  - G115 ×2 (`health-resources_linux.go`, converting `statfs` `Bsize` from
    int64 to uint64): pre-existing. The kernel's block size is positive, so
    accepted.
  - G204 (`CheckOptions.command`): tool names are constants, and the only user
    input (`--service`) is validated and follows `--` (SEC-001).
  - G304 (`createDatabaseFile`): the path is the user's own `SALUS_DB_PATH` or
    per-user default, by design.
  - *Update 2026-09-28 (v2.29.0 on the M5 working tree):* six findings. The
    two new ones are G703 (path traversal via taint analysis, HIGH):
    `os.Stat` in `ownerOnly` and `worldWritablePath` (`../pkg`).
    Both are false positives:
    - The paths come from the invoking user's own `SALUS_DB_PATH`,
      `KUBECONFIG`, `PATH`, and home directory.
    - They are only stat'ed for permission bits, never opened.
    - No privilege boundary is crossed.

    Following Q-009, they are triaged (dismissed as false positives) in Code
    Scanning, not suppressed in code.
  - *Update 2026-10-03 (M6, gosec not run):* expect new G304 findings (file
    path from a variable) for `os.Open` in `readCertificates`
    (`health-certs.go`) and `sshdParser.parseFile` (`health-sshd.go`), and
    possibly G302 for `tmp.Chmod(mode)` in `writeFileAtomic`. The paths are
    the invoking user's own `--cert` values, `SALUS_SSHD_CONFIG` or the
    standard sshd location and its `Include` files, and `--output`; files are
    opened read-only (or created with `os.CreateTemp`), and the mode is
    `0600` or the existing file's own. Triage them in Code Scanning like the
    G703 findings.
- **Container image (P2-3, 2026-09-27).**
  - `golang:1.26-alpine` had moved to Alpine 3.24 while the runtime stage was
    `alpine:3.22`, so the binary was linked against a newer musl than it ran
    on. Both stages are now pinned to 3.24 (`golang:1.26-alpine3.24`,
    `alpine:3.24`). `golang:1.26-alpine3.22` was not used because it had not
    been rebuilt since June 2026 and would ship an older Go.
  - The runtime stage no longer installs `sqlite-libs` or `ca-certificates`.
    go-sqlite3 compiles its bundled SQLite unless built with the `libsqlite3`
    tag, and Salus makes no TLS connections. This was established from the
    source rather than `ldd`; the Docker smoke tests confirm the binary runs.
  - `hadolint` v2.15.1 reports only DL3018 for the builder's
    `apk add build-base` (no version pin). This is accepted: Alpine drops old
    package versions, so pins would break rebuilds. The base-image digest pins
    already fix the package set.
  - **Go patch version (SEC-008).** CI and CD installed Go 1.26.0, because
    `go.mod` said `go 1.26.0` and `setup-go` installs exactly that version
    (CI log for run 36367840814: `Setup go version spec 1.26.0`,
    `go version go1.26.0`). v1.0.0 release archives were therefore built with
    1.26.0, while the Docker image was built with the builder's 1.26.8.
    `go.mod` is now `go 1.26.8` (released 2026-09-01; the
    `golang:1.26-alpine3.24` image is on 1.26.8 with `GOTOOLCHAIN=local`).
  - The image now runs as UID 10001. A Docker volume created by 1.0.0 holds a
    root-owned `salus.db` that this user cannot write, so it needs the
    one-time `chown` in the README.
  - No Docker daemon or registry access was available in the analysis
    environment, so the image was not built locally. The Docker workflow is
    the first build.
- **`?` in database paths.** go-sqlite3 treats text after the first `?` of a
  plain path as connection parameters. `databaseFile` mirrors this, so file
  creation and the permission check target the real file.
- **`--quiet` with `--json`** printed JSON. *Resolved 2026-09-27 (P1-6):*
  `--quiet` wins and suppresses report output. This was chosen over making the
  flags mutually exclusive, which would have turned the combination into an
  error.
- **Duplicated constants.** *Resolved 2026-09-27 (P1-3):* `DatabasePathEnv`
  and `DefaultDatabasePath` are defined once in `internal/database.go`.
- **Non-Linux results.** *Resolved 2026-10-04 (P3-7):* disk, inode, memory,
  CPU, and host uptime checks measure macOS and Windows hosts. `check run`
  still exits `1` there with default options, because the systemd-based
  checks (`service-uptime` with `--service`, `systemd-failed`, `time-sync`)
  report WARN off Linux.
- **macOS and Windows data sources (P3-7, 2026-10-04).**
  - macOS memory is computed from VM page counts: available = free +
    speculative + file-backed (`vm.page_pageable_external_count`) +
    purgeable pages, over `hw.memsize / vm.pagesize`. That is close to
    Activity Monitor's "Memory Used" and to Linux's `MemTotal -
    MemAvailable`. The first draft used `kern.memorystatus_level` (what
    `memory_pressure(1)` prints); the independent review showed from XNU
    source that on macOS it counts active pages as available, so it measures
    pressure (wired and compressed memory), not use, and a Mac with apps
    holding most of its memory would have passed. The kernel page size
    (`vm.pagesize`) is used because `hw.pagesize` reports 4096 to an amd64
    binary under Rosetta. Integer sysctls are read as 4 or 8 bytes
    (`darwinSysctlUint`), because XNU declares some as int and some as quad.
  - macOS swap (`vm.swapusage`) is allocated on demand, so a host with no
    swap file yet reports 0%.
  - The sysctl struct layouts (`loadavg` 24 bytes, `xsw_usage` 32,
    `timeval` 16) are for LP64 little-endian macOS (amd64 and arm64).
  - Windows has no load average; a 1-second `GetSystemTimes` sample stands
    in, so `check run` takes about a second longer there, and a short burst
    can WARN where a 1-minute average would not.
  - Windows "swap" is reported as the commit charge against the commit
    limit (physical memory plus page files), from `GlobalMemoryStatusEx`.
  - On Windows, `--disk-path /` (the default) is the root of the current
    drive; `filepath.FromSlash` turns `/` into `\` before
    `GetDiskFreeSpaceExW`. That function takes only directories, and UNC
    shares need a trailing backslash, so a file path is replaced by its
    directory and a separator is appended.
  - `GetTickCount64` returns its 64-bit result in two registers on 32-bit
    Windows; both halves are combined there (windows/386 is not a release
    target, but it builds).
  - None of this ran on a real Mac or Windows machine before commit; the
    CI jobs on `macos-latest` and `windows-latest` run
    `TestResourceChecksReadThisHost`, `TestDarwinSysctlsReadable`, and
    `TestWindowsKernel32Readable` against the real system calls. They passed
    in CI #80 on `03964ff` (2026-10-04). Those tests have no skip path, so a
    green run means they ran.
- **Windows false positive in `misconfig`.** On Windows, Go's `FileMode`
  reports `0666` for any file without the read-only attribute
  (`os/types_windows.go`), so the "writable by group/other" test fires for
  every existing database whenever `SALUS_DB_PATH` is set. This is inferred
  from the Go source and has not been observed on a Windows host.
  *Resolved 2026-09-27 (P1-5):* the POSIX permission test is skipped on
  Windows. Windows ACLs are not checked (see SEC-004).
- **Multi-line messages.** *Resolved 2026-09-27 (P1-7):* `service-uptime` uses
  only the first line of `systemctl` output. Still open: on success,
  `docker-status` embeds the full combined output of `docker info`, so stderr
  warnings could make that message multi-line. *Resolved 2026-09-28 (P3-2):*
  `dockerServerVersion` takes the first line that is not blank and not a
  `WARNING`. Since SEC-009, `RunChecks` also replaces any control character,
  including newlines, in every message.
- **Running inside containers.** `/proc/meminfo` and `/proc/loadavg` report
  host-wide values, not cgroup limits, and `disk-space` measures the
  container filesystem unless a host path is mounted and passed with
  `--disk-path`.
- **Thresholds are not configurable from the CLI.** `CheckOptions` supports
  warn/fail thresholds and a command timeout, but only `--disk-path` and
  `--service` are exposed as flags. *Resolved 2026-09-27 (P3-1):* `check run`
  has `--disk-warn`, `--disk-fail`, `--mem-warn`, `--mem-fail`, `--load-warn`,
  `--load-fail`, and `--timeout`, validated by `validateLimits` before
  anything runs. The defaults moved from `health-thresholds.go` (Linux-only)
  to `health.go`, because the flags need them on every platform. The flags
  are accepted on macOS and Windows, where the resource checks still report
  WARN until P3-7.
- **Only reachability is checked for Docker.** The "Container Runtime"
  category description in `seed.go` mentions container health, but
  `docker-status` checks only daemon reachability. *Resolved 2026-09-28
  (P3-2):* unhealthy or restarting containers make it WARN.
- **Container and node queries (P3-2, P3-3).**
  - Unhealthy containers come from `docker ps --filter health=unhealthy`, which
    lists only running containers, so a stopped container with a stale health
    status is not reported.
  - Restarting containers come from
    `docker ps --all --filter status=restarting`.
  - Node readiness comes from `kubectl get nodes` with a JSONPath that prints
    each node's `Ready` condition.
  - A Forbidden error is detected in the output text. It keeps the check PASS,
    because namespace-scoped users cannot list nodes.
  - Each query has its own `--timeout`. `docker-status` runs up to three
    commands (`info` and two listings), and `kubernetes-status` up to two, so
    a check can take up to three or two timeouts.
  - `kubectl cluster-info` lists Services in `kube-system`, which
    namespace-scoped users may not do. It prints its "To further debug…" hint
    on stdout before the error. A Forbidden answer therefore counts as
    reachable, and `errorLine` skips the hint and kubectl's log lines, as
    noted by the independent review of 2026-09-28.
- **WSL (review finding, 2026-09-28).**
  - WSL mounts Windows drives with drvfs (`/mnt/c`): type `drvfs` on WSL 1,
    and `9p` with `aname=drvfs` on WSL 2. Their mode bits are made up,
    commonly 0777.
  - WSL also appends the Windows `PATH` by default. `path-world-writable`
    would otherwise warn on every default WSL host, and a kubeconfig or
    database kept on a Windows drive would trip the permission rules.
  - `syntheticModes` reads `/proc/self/mounts`, and the permission rules skip
    such paths. It is not verified on a WSL host; the tests use mount
    fixtures.
- **Per-target results (M6, P6-1).** A targeted check still examines one
  thing per call; `RunChecks` expands it. Text output and `jobs show` text
  print only the key, so messages of targeted checks must name their target
  (they already did: `/var: …`, `service "nginx" …`, `<cert path>: …`).
  Nagios detail lines and `jobs diff` follow the text report; only Nagios
  performance-data labels and JUnit test names add the target, because they
  must be unique. `--disk-path` and `--cert` are string arrays, because paths
  may contain commas; `--service` is a string slice, because unit names cannot.
- **`--output` and node_exporter (M6, P6-2).** The temporary file is
  `.<name>.<random>.tmp` in the target directory, which the textfile
  collector ignores (it reads `*.prom`), and the rename is atomic within one
  filesystem. A new file is `0600`, which node_exporter (usually its own user)
  cannot read; the README tells operators to create the file once with the
  mode they need, which later writes keep. `os.Stat` follows a symlink at the
  target for the mode, and the rename replaces the symlink itself.
- **sshd_config semantics (M6, P6-10).** For `PermitRootLogin` and
  `PasswordAuthentication`, sshd keeps the first value it reads. `Include`
  is processed in place, and its files are read in lexical order (glob(3),
  whose wildcards do not match a leading dot). sshd saves and restores the
  Match state around each included file, so a `Match` inside an included file
  ends with that file. A `Match` line makes the following lines conditional
  (including `Include` lines) until a `Match all` line, which sshd applies at
  startup, makes them global again. Relative `Include` paths are resolved
  against the configuration file's directory, which equals OpenSSH's
  `/etc/ssh` for the default location. For a `SALUS_SSHD_CONFIG` outside
  `/etc/ssh` (a host's configuration mounted elsewhere), absolute includes
  under `/etc/ssh/` are read from that directory instead, and any other
  absolute include makes the rules stay silent. Quoted values may use `"` or
  `'`; backslash escapes are not interpreted. On Ubuntu, `sshd_config.d/50-cloud-init.conf` is often root-only, so
  a non-root run reports nothing for the sshd rules rather than a possibly
  wrong default. `KbdInteractiveAuthentication` with `UsePAM yes` can also
  accept passwords; it is not evaluated.
- **New checks in the default run (Q-012).** On hosts without systemd
  (containers, WSL 1, macOS, Windows), `systemd-failed` and `time-sync`
  report WARN, like the other Linux-only checks; without `kubectl`,
  `kubernetes-pods` reports WARN like `kubernetes-status`. The CI, CD, and
  Docker smoke steps use `--only`, so they are unaffected.
- **Seeded catalog text is insert-only.** Seeding uses `FirstOrCreate`
  (`firstOrInsert` since P7-1), so changes to names or descriptions in `seed.go` reach only
  new databases. Updating rows on every open would break read-only databases.
  M5 left the `docker-status` and `kubernetes-status` descriptions
  ("reachable") as they were. M6 updated them in `seed.go` (and the
  `disk-space` and `service-uptime` descriptions, for multiple targets), so
  new databases list the full behavior while databases created earlier keep
  the old text in `check list`; the README describes the behavior either way.
- **Pruning and time zones (P3-6).** Stored `started_at` values keep the UTC
  offset they were written with, and the text sorts correctly only within one
  offset. Since M6, `ListScanJobs` (used by `jobs list` and the default
  `jobs diff` pair) and `ScanStats` also order by `julianday(started_at)`; the
  independent review showed the text order picking the wrong pair across a
  DST change. `PruneScanJobs` compares with `julianday`, and a test mixes UTC+14
  and UTC-8 rows. Pruning frees pages inside the SQLite file but does not
  shrink it; `VACUUM` was not added.
- **Test depending on the host.** *Resolved 2026-09-27 (P1-8):* external tools
  are faked in tests (`fakeToolOptions`), and no test runs `docker`,
  `kubectl`, or `systemctl`. On Linux, `TestRunChecksDefaultsToAllChecks`
  still reads the real `statfs("/")`, `/proc/meminfo`, `/proc/loadavg`, and
  `/proc/uptime` through `RunChecks`, but only asserts on keys.
- **Runtime errors printed usage text.** Cobra printed the full usage after
  any error. Once `main.run` gave Cobra the stdout writer, that usage (and the
  error) went to stdout, which would have corrupted `--json` output; an
  independent review caught this before commit. *Resolved 2026-09-27 (P1-11):*
  Cobra's printing is silenced, and `main.run` writes `Error: …` plus a
  `--help` hint to stderr.
- **Mistyped subcommands exited 0.** `salus check rn` printed help and exited
  `0`, because Cobra treats extra arguments to a non-runnable group command as
  a help request. Leaf commands also silently ignored extra arguments.
  *Resolved 2026-09-27 (P1-11):* `runGroup` rejects unknown subcommands with
  exit `3`, and leaf commands declare `Args`.
- **GORM logged to stdout.** `jobs show <missing id>` printed GORM's
  default logger line (source path, SQL, colors) to stdout, and slow-query
  warnings could corrupt `--json`. *Resolved 2026-09-27 (P1-11):* the GORM
  logger is silent.

### Unattended-run hardening (M7, 2026-10-04)

- **Abandoned checks.** `runCheck` gives up on a check that passes its time
  limit, but Go cannot stop a goroutine blocked in a system call. The
  goroutine stays until the system call returns or the process exits, so a
  hung NFS mount can leave one goroutine (and its thread) per affected
  target for the rest of the run. Checks must not write shared state.
- **Signals and process groups.** Tools run in their own process group on
  Unix (P7-3), so a terminal's Ctrl-C reaches only Salus, which then kills
  the groups. A tool that reads `/dev/tty` (an ssh passphrase prompt for an
  `ssh://` `DOCKER_HOST`) is stopped by the terminal and times out. With the
  default 3s `--timeout`, such prompts were not practical before either.
- **Signal races.** When systemd stops a service it signals the whole control
  group, so a tool can die from the same SIGTERM before Salus's handler
  cancels the run. `runExternal` waits up to `stopSignalGrace` (250ms) for
  that cancellation when a tool died from SIGINT or SIGTERM, and `check run`
  treats a context cancelled by the time the checks return as an
  interruption. In 160 stress runs (GOMAXPROCS 1 and 4, signal to Salus or
  to the tool first) every run exited 3 without a report.
- **Windows.** Only the tool itself is killed on timeout; processes it
  started can outlive it. `WaitDelay` still bounds how long Salus waits for
  them. A job object would be needed to kill the tree.
- **Fatal runtime errors** (out of memory, concurrent map writes) cannot be
  recovered and still exit 2. The 8 MiB output limit removes the one known
  path to unbounded memory.

### GitHub Actions maintenance (observed 2026-09-27)
- **Node 20 deprecation sources.** Run annotations on `78db94e` and v1.0.1
  flagged these actions, each with its fix:

  | Action | Fix |
  |---|---|
  | `actions/checkout` v4.4.0 | Dependabot #21 → v7.0.1 |
  | `actions/setup-go` v5.6.0 | #13 → v7.0.0 |
  | `github/codeql-action` v3.38.1 | #18 → v4.38.2, all four sub-actions |
  | `actions/upload-artifact` v4.6.2 and `download-artifact` v4.3.0 | #19 → v7.0.1 and v8.0.1 |
  | `actions/github-script` v7.0.1, run inside `codecov/codecov-action` v5.5.5 | #20 → Codecov v7.1.1, which uses `github-script` v8.0.0 |
  | `softprops/action-gh-release` v2.6.2 | local change to v3.0.3 |

  `golangci/golangci-lint-action` v9.3.0 and `securego/gosec` (a Docker
  action) are not flagged. Dependabot had not proposed softprops because five
  PRs were already open, its default limit per ecosystem.
- **Major-version notes checked for our usage:**
  - checkout v6 stores credentials in a separate file, and v7 refuses fork
    checkouts under `pull_request_target` and `workflow_run`. No workflow here
    pushes or uses those triggers.
  - setup-go v6 changed toolchain selection, but v7 still installs exactly Go
    1.26.8 from `go.mod` (PR #13 logs).
  - upload-artifact v7 adds opt-in unzipped uploads, so the default is
    unchanged.
  - download-artifact v8 fails on digest mismatches by default and skips
    unzipping non-zip files. Our `pattern` plus `merge-multiple` usage is
    unaffected.
  - Codecov v7 changed only its signing key account.
  - softprops v3 changes only the runtime.
- **Rulesets.**
  - `BestBranch` applies to every branch. It requires a pull request with one
    code-owner approval, allows only merge commits, and requires CodeQL code
    scanning to pass. It also blocks deletion and non-fast-forward pushes.
  - `BestTag` blocks tag deletion and force pushes, so a failed release tag
    cannot be reused.
  - The repository admin role and some GitHub Apps can bypass both. That is why
    the Dependabot PRs show `BLOCKED` until they are approved.
- **Tag placement (2026-09-28).**
  - v1.0.2 was tagged at `08b2faa`, before the Dependabot PRs were merged at
    `2fd2496`.
  - CD #3 therefore still used `checkout` v4.4.0, `setup-go` v5.6.0, and the
    v4 artifact actions for the binaries. Its build jobs carry Node 20
    warnings, and the bumped pins have not run in CD yet.
  - When a release should exercise workflow updates, merge them before
    tagging, or run CD manually on `main` first.
- **Ubuntu 26.04.** `ubuntu-latest` moves to Ubuntu 26.04 in a rollout from
  2026-10-19 to 2026-11-19 (actions/runner-images#14748). During the rollout a
  job may land on either image. CD is pinned to 24.04 (Q-011).

### Documentation drift observed
- The README install section referred to raw binaries named
  `salus_<os>_<arch>`, while `cd.yml` publishes `.tar.gz`/`.zip` archives
  (named `munus_*` at `7235211`). Resolved 2026-09-27: archives are canonical
  (Q-003), and the README install section now describes them and checksum
  verification.
- The README has no build-from-source, prerequisites (Go 1.26 plus a C
  toolchain for CGO), configuration, testing, or project-structure sections,
  all of which `AGENTS.md` lists for the README. *Resolved 2026-09-27 (P4-1):*
  the README follows the `AGENTS.md` section order and adds use cases,
  prerequisites, build from source, provenance verification, an exit-code
  table, testing and quality checks, and the project structure. macOS
  Gatekeeper guidance is still missing, because it needs a check on a Mac.
- `NOTICE` ended with "This product includes third-party software:" and an
  empty list after `7235211`. Resolved 2026-09-27: the list now matches the
  modules linked into the binary (`go list -deps` for linux, darwin, and
  windows), with licenses taken from each module's license file.
- `CONTRIBUTING.md` was emptied in `7235211`. It was restored from history and
  expanded on 2026-09-27 (see `history.md`).
- `AGENTS.md` referred to "`CONTRIBUTING.md `" with a trailing space in two
  places. Cosmetic. Resolved: no longer present (checked 2026-10-05).

### Verification environment (2026-09-27 local session)
A later session on the maintainer's workstation had Go 1.26.8, network access
to `proxy.golang.org` and `vuln.go.dev`, and an authenticated GitHub CLI.
golangci-lint v2.13.2, actionlint 1.7.12, and govulncheck v1.8.0 were built
from source into a scratch directory. The SEC-003 build-context check, manual
workflow runs, Code Scanning alerts, and branch protection settings could not
be checked from it; they are listed as maintainer steps in `plan.md`.

On 2026-09-28, to validate SEC-006, GitHub CLI 2.101.0 was built from source
and run with its own configuration directories. `gh attestation verify` needs
a GitHub login even for a public repository (exit 4 without one).

### Verification environment (2026-09-27 analysis)
The analysis environment could not reach `proxy.golang.org` or `go.dev`. The
baseline in `plan.md` was established by building Go 1.26.8 from its GitHub
source and resolving each module from its upstream GitHub repository at the
exact version in `go.mod`, wired in with local `replace` directives in a
throwaway copy. This differs from a normal build in one way: module content
was not verified against `go.sum` or the checksum database. Treat the result
as strong evidence, and treat GitHub Actions results as authoritative.

## Open questions and decisions

Decisions recorded 2026-09-27 from the maintainer (Q-012 to Q-014 on 2026-10-03, Q-015 to Q-018 on 2026-10-04).

| ID | Question | Why it matters | Decision |
|---|---|---|---|
| Q-001 | Is a TUI still planned, for example with Bubble Tea, or should the empty `logic-tui.go` and `ui-form.go` be removed? | Determines whether new dependencies are expected and whether the placeholders are dead code. | **No TUI. Salus is a pure CLI.** Remove the placeholders (P5-2). |
| Q-002 | Should the default database stay at `./salus.db`, or move to a per-user data directory (for example `$XDG_DATA_HOME/salus/salus.db`)? | Changing it is a behavior change. It affects SEC-004 and cron/CI usage. | **Per-user data directory.** `SALUS_DB_PATH` still overrides. Implemented 2026-09-27 (P1-10), after v1.0.0. |
| Q-003 | What is the canonical release format: raw binaries as the README describes, or archives as `cd.yml` produces? | README install steps and CD packaging must agree. | **Archives, as `cd.yml` produces.** README updated. |
| Q-004 | Should operational errors use a distinct exit code (for example `3`) instead of sharing `1` with WARN? | Changes the public exit-code contract, but makes Salus reliable in scripts. | **Yes.** Distinct exit code (P1-9). |
| Q-005 | Should disk, memory, and CPU checks be implemented for macOS and Windows, or documented as Linux-only (and possibly reported as skipped instead of WARN)? | Cross-platform support likely needs `golang.org/x/sys` or per-OS syscalls. The status choice affects exit codes. | **Implement for macOS and Windows** (P3-7). |
| Q-006 | Were the removal of `CONTRIBUTING.md` content and the truncation of `NOTICE` in `7235211` intentional? | `CONTRIBUTING.md` was restored on that assumption. `NOTICE` was left untouched pending an answer. | **Yes, intentional, so that correct data could be filled in.** `CONTRIBUTING.md` (2026-09-27, `460a24b`) and `NOTICE` (P0-5) now hold verified content. |
| Q-007 | Should `.idea/` be ignored (the `.gitignore` line is commented out) or partially tracked? | `.idea/` shows as untracked and includes per-user `workspace.xml`. | **Track.** Done in `460a24b`. `.idea/.gitignore` keeps `workspace.xml` and other per-user files out. |
| Q-008 | Should CI keep triggering on both `push` to every branch and `pull_request` to every branch? | Same-repo PR branches run CI twice. | **Yes, keep both triggers.** No change. |
| Q-009 | Should gosec findings gate merges, and at what severity? | See SEC-005. | **No.** gosec stays non-blocking. Findings are triaged in GitHub Code Scanning. |
| Q-010 | For SEC-006, is GitHub's immutable-release attestation (observed on v1.0.1, verifiable with `gh release verify-asset`) enough, or should releases also get build provenance attestations? | The release attestation shows an asset belongs to the release and was not changed afterwards. It does not show that the workflow built the asset. Build provenance adds `id-token: write` and `attestations: write` to the `release` job. | **Add build provenance** (2026-09-27). `actions/attest` runs in a CD `package` job that uses only first-party actions, on tag and manual runs; a separate `release` job publishes (P2-6, SEC-006). |
| Q-011 | Should CI and CD pin `ubuntu-24.04` instead of `ubuntu-latest`, which moves to Ubuntu 26 from 2026-10-19? | Pinning keeps release builds reproducible, but needs manual bumps. Staying on `latest` needs a CD `workflow_dispatch` run after the switch and before the next tag (P2-8). | **Pin CD only** (2026-09-27). CD's linux/amd64 build and release job use `ubuntu-24.04`, matching linux/arm64 on `ubuntu-24.04-arm`. CI, Security, and Docker stay on `ubuntu-latest` for early warning. |
| Q-012 | Should the M6 checks (`disk-inodes`, `systemd-failed`, `time-sync`, `kubernetes-pods`, `cert-expiry`) run in a plain `check run`? | Adding checks to the default run can raise exit codes on hosts that passed before. | **Yes** (2026-10-03). They join the default run; `cert-expiry` runs only when `--cert` is given. The changes are listed under "Upgrading from 1.0.2" (P6-5 to P6-9). |
| Q-013 | Should `sshd-password-auth` warn only on an explicit `PasswordAuthentication yes`, or on the effective value, which defaults to yes when unset? | The effective value catches stock installs, so more hosts warn. | **Effective value** (2026-10-03). An unset keyword counts as yes, as in OpenSSH (P6-10). |
| Q-014 | What severity for pods in CrashLoopBackOff or not Ready, and for nodes reporting Memory, Disk, or PID pressure? | Severity decides exit codes and alerting. | **WARN** (2026-10-03), matching the P3-2 decision for unhealthy containers. FAIL stays reserved for an unreachable cluster or no Ready node (P6-9). |
| Q-015 | When `check run` has written its report but saving the run fails, which exit code? | Scripts and Nagios read the code; the report is the monitoring signal. | **Exit 3** (2026-10-04), as before, but the report is now written first (P7-2). Nagios then shows the check state with UNKNOWN. |
| Q-016 | `check run --json` is a bare array without version or metadata. Keep it, add an envelope format, or switch to an envelope? | An envelope later is a breaking change for consumers. | **Keep it and document stability** (2026-10-04): fields are only added, never renamed or removed (P7-8). |
| Q-017 | What status for a check stopped by `--check-timeout` or `--run-timeout`? | Severity decides exit codes and alerting. | **FAIL** (2026-10-04): a hang usually means a hung mount, daemon, or tool, which matches "FAIL when unreachable" (P7-4). |
| Q-018 | What should `check run` do on SIGINT or SIGTERM? | systemd and terminals stop runs this way; the exit code and the saved history must stay truthful. | **Stop, save nothing, exit 3** (2026-10-04): kill running tools, write no report, record no run (P7-5). |
