# Engineering Notes and Open Questions

Durable engineering notes and unresolved technical questions. Active work items
live in [`plan.md`](plan.md), security items in [`cybersec.md`](cybersec.md).

Last reviewed: 2026-09-27 (against commit `7235211`).

## Engineering notes

### Lineage
- Salus was converted from an earlier project named "Rete" on 2026-09-03/04
  (commits `3a67f29`, `8d90da9`, `3b752f2`). Remnants:
  - the `Dockerfile` comment "Persist sqlite database file (rete.db)";
  - the `Dockerfile` comment "This app is an interactive TUI/CLI". Salus has
    no TUI today;
  - empty `internal/logic-tui.go` and `internal/ui-form.go`;
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

At `7235211` this commit is local only (`main` is one commit ahead of
`origin/main`). If pushed as-is, the CI smoke step, the Docker smoke step, and
the CD smoke step will fail. See Phase 0 in `plan.md`.

The CGO build strategy in `cd.yml` fits Salus and should be kept when
renaming: native runners per OS, pinned llvm-mingw for windows/arm64, and
static Linux linking with `sqlite_omit_load_extension,osusergo,netgo`.

### Behavior worth knowing
- **Exit code 1 means two things.** `check run` exits `1` for WARN, and `main`
  also exits `1` for any Cobra error (unknown `--only` key, DB failure,
  bad flag). Scripts cannot tell "warning" from "Salus failed to run."
- **`os.Exit` inside `check run`.** The command exits from within `RunE`, which
  skips deferred cleanup and prevents `check run` from being tested through
  `cmd.Execute()`. The DB handle is never closed explicitly.
- **Job timestamps.** `RecordScan` sets `StartedAt` after all checks have
  finished, so `StartedAt` and `FinishedAt` are nearly identical and do not
  reflect check duration. The `running` and `failed` job statuses are never
  visible: the job is written and completed in one transaction, and a
  failed transaction leaves no row.
- **Read outside the transaction.** `RecordScan` calls `featureByKey(db, ...)`
  with the outer handle inside `Transaction`. It works with SQLite's default
  locking (readers are allowed while the writer holds a RESERVED lock), but
  would deadlock if the pool were limited to one connection, and it reads
  outside the transaction's snapshot.
- **DB is opened for every command,** including `--help`, `check list`, and
  `check run --no-save`, so `salus.db` is created in the current directory
  even when nothing is persisted.
- **`--quiet` with `--json`** still prints JSON, because JSON takes precedence
  in the `switch`.
- **Duplicated constants.** `SALUS_DB_PATH` and the default `salus.db` are
  defined in both `database_path.go` (package `main`) and
  `internal/database.go`, which invites drift.
- **Non-Linux results.** Disk, memory, and CPU checks return `WARN` on macOS
  and Windows, so `check run` exits `1` there with default options.
- **Windows false positive in `misconfig`.** On Windows, Go's `FileMode`
  reports `0666` for any file without the read-only attribute
  (`os/types_windows.go`), so the "writable by group/other" test fires for
  every existing database whenever `SALUS_DB_PATH` is set. This is inferred
  from the Go source and has not been observed on a Windows host.
- **Multi-line messages.** In the failure branch, `service-uptime` embeds the
  whole trimmed combined stdout/stderr of `systemctl`, not just the first
  line, so messages can span several lines and break the aligned text report.
- **Running inside containers.** `/proc/meminfo` and `/proc/loadavg` report
  host-wide values, not cgroup limits, and `disk-space` measures the
  container filesystem unless a host path is mounted and passed with
  `--disk-path`.
- **Thresholds are not configurable from the CLI.** `CheckOptions` supports
  warn/fail thresholds and a command timeout, but only `--disk-path` and
  `--service` are exposed as flags.
- **Only reachability is checked for Docker.** The "Container Runtime"
  category description in `seed.go` mentions container health, but
  `docker-status` checks only daemon reachability.
- **Test depending on the host.** `TestRunChecksDefaultsToAllChecks` runs real
  checks, including `docker`, `kubectl`, and `systemctl` with 3s timeouts
  when they are installed. It asserts only on keys and order.

### Documentation drift observed
- The README install section refers to raw binaries named `salus_<os>_<arch>`,
  while `cd.yml` publishes `.tar.gz`/`.zip` archives, and at `7235211` they
  are named `munus_*`.
- The README has no build-from-source, prerequisites (Go 1.26 plus a C
  toolchain for CGO), configuration, testing, or project-structure sections,
  all of which `AGENTS.md` lists for the README.
- `NOTICE` ends with "This product includes third-party software:" and an
  empty list after `7235211`.
- `CONTRIBUTING.md` was emptied in `7235211`. It was restored from history and
  expanded on 2026-09-27 (see `history.md`).
- `AGENTS.md` refers to "`CONTRIBUTING.md `" with a trailing space in two
  places. Cosmetic.

### Verification environment (2026-09-27 analysis)
The analysis environment could not reach `proxy.golang.org` or `go.dev`. The
baseline in `plan.md` was established by building Go 1.26.8 from its GitHub
source and resolving each module from its upstream GitHub repository at the
exact version in `go.mod`, wired in with local `replace` directives in a
throwaway copy. This differs from a normal build in one way: module content
was not verified against `go.sum` or the checksum database. Treat the result
as strong evidence, and treat GitHub Actions results as authoritative.

## Open questions

| ID | Question | Why it matters | Owner |
|---|---|---|---|
| Q-001 | Is a TUI still planned, for example with Bubble Tea, or should the empty `logic-tui.go` and `ui-form.go` be removed? | Determines whether new dependencies are expected and whether the placeholders are dead code. | Maintainer |
| Q-002 | Should the default database stay at `./salus.db`, or move to a per-user data directory (for example `$XDG_DATA_HOME/salus/salus.db`)? | Changing it is a behavior change. It affects SEC-004 and cron/CI usage. | Maintainer |
| Q-003 | What is the canonical release format: raw binaries as the README describes, or archives as `cd.yml` produces? | README install steps and CD packaging must agree. | Maintainer |
| Q-004 | Should operational errors use a distinct exit code (for example `3`) instead of sharing `1` with WARN? | Changes the public exit-code contract, but makes Salus reliable in scripts. | Maintainer |
| Q-005 | Should disk, memory, and CPU checks be implemented for macOS and Windows, or documented as Linux-only (and possibly reported as skipped instead of WARN)? | Cross-platform support likely needs `golang.org/x/sys` or per-OS syscalls. The status choice affects exit codes. | Maintainer |
| Q-006 | Were the removal of `CONTRIBUTING.md` content and the truncation of `NOTICE` in `7235211` intentional? | `CONTRIBUTING.md` was restored on that assumption. `NOTICE` was left untouched pending an answer. | Maintainer |
| Q-007 | Should `.idea/` be ignored (the `.gitignore` line is commented out) or partially tracked? | `.idea/` shows as untracked and includes per-user `workspace.xml`. | Maintainer |
| Q-008 | Should CI keep triggering on both `push` to every branch and `pull_request` to every branch? | Same-repo PR branches run CI twice. | Maintainer |
| Q-009 | Should gosec findings gate merges, and at what severity? | See SEC-005. | Maintainer |
