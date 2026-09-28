# Salus

Salus is an environment health checker CLI. It verifies disk space, memory, CPU
load, Docker status, Kubernetes status, service uptime, and common
misconfigurations, then reports a concise PASS/WARN/FAIL summary.

It is built for operators, cron jobs, and CI pipelines that need a quick,
scriptable view of a host's health: results are printed as text or JSON, the
exit code reflects the worst result, and each run is recorded in a local
SQLite database for later review (unless you pass `--no-save`).

## Features

- **System Resources**
  - Disk space usage on a configurable mount point
  - Memory and swap usage
  - CPU load average relative to available CPUs
  - Configurable WARN and FAIL thresholds for all three

- **Container Runtime**
  - Docker daemon reachability

- **Orchestration**
  - Kubernetes cluster reachability via `kubectl`

- **Services**
  - systemd service uptime (or host uptime when no service is specified)

- **Configuration**
  - Common environment misconfigurations (`HOME` not set, a database file that
    other users can read or write)

- **Job Tracking**
  - Every `check run` is recorded as a job in a local SQLite database
  - Browse past runs and their individual results

- **Scripting**
  - Text or JSON output, and exit codes that reflect the worst result

Platform support: the disk space, memory, CPU load, and host uptime checks
read `/proc` and `statfs`, so they run on Linux only. On macOS and Windows they
report `WARN` (for example `disk space check is only supported on Linux`), so
a full `check run` there exits with code `1` or higher. Checking a service
with `--service` requires systemd, so it is also Linux-only.

## Use cases

- **Scheduled host checks.** Run `salus check run --quiet` from cron, alert on
  a non-zero exit code, and review past runs with `salus jobs list` and
  `salus jobs show <id>`.
- **CI or deployment gates.** Check a build or deployment host before a
  pipeline step, for example
  `salus check run --only disk-space,memory --disk-fail 95 --json`. Treat exit
  code `2` (FAIL) as fatal and `1` (WARN) as a warning, and keep the JSON
  output with the build.
- **Service verification.** Confirm that a systemd unit is active after a
  deployment: `salus check run --only service-uptime --service nginx`.
- **Workstation checks.** Before working with containers or a cluster, confirm
  that the Docker daemon and the current `kubectl` context respond:
  `salus check run --only docker-status,kubernetes-status --no-save`.

## Prerequisites

To run a release binary:

- Linux, macOS, or Windows on x86-64 or ARM64 (see
  [Installation](#installation)).
- Optional tools, used by individual checks when they are on `PATH`: the
  `docker` CLI (`docker-status`), `kubectl` with a configured context
  (`kubernetes-status`), and `systemctl` (`service-uptime` with `--service`,
  Linux only). A missing tool makes its check report `WARN`.

To build from source:

- **Go 1.26.8** or newer, matching the `go` directive in `go.mod`.
- **A C toolchain** (for example `gcc` or `clang`) with `CGO_ENABLED=1`. The
  SQLite driver `github.com/mattn/go-sqlite3` uses CGO. A build with CGO
  disabled compiles but cannot open its database.
- **Git.**
- Optional: **GNU Make** for the `make` targets, **golangci-lint v2.13.2** (the
  version CI uses), and **Docker** to build the container image.

## Installation

### Release archives

Release archives and a `checksums.txt` file are published on the
[GitHub Releases](https://github.com/jabbott-iii/Salus/releases) page for each
version tag. Each archive contains a single binary with the same base name.

| Platform | Archive |
|---|---|
| Linux (x86-64) | `salus_linux_amd64.tar.gz` |
| Linux (ARM64) | `salus_linux_arm64.tar.gz` |
| macOS (Apple Silicon) | `salus_darwin_arm64.tar.gz` |
| macOS (Intel) | `salus_darwin_amd64.tar.gz` |
| Windows (x86-64) | `salus_windows_amd64.zip` |
| Windows (ARM64) | `salus_windows_arm64.zip` |

Download the archive for your platform and `checksums.txt` into the same
directory, then verify, extract, and install it.

Linux (use `linux_arm64` in place of `linux_amd64` on ARM64):
```bash
grep salus_linux_amd64.tar.gz checksums.txt | sha256sum --check
tar -xzf salus_linux_amd64.tar.gz
sudo mv salus_linux_amd64 /usr/local/bin/salus
salus --version
```

macOS (use `darwin_amd64` in place of `darwin_arm64` on Intel Macs):
```bash
grep salus_darwin_arm64.tar.gz checksums.txt | shasum -a 256 --check
tar -xzf salus_darwin_arm64.tar.gz
sudo mv salus_darwin_arm64 /usr/local/bin/salus
salus --version
```

Windows, in PowerShell (use `windows_arm64` in place of `windows_amd64` on ARM64):
```powershell
# Compare this hash with the salus_windows_amd64.zip line in checksums.txt
(Get-FileHash .\salus_windows_amd64.zip -Algorithm SHA256).Hash.ToLower()
Expand-Archive .\salus_windows_amd64.zip -DestinationPath .
Rename-Item .\salus_windows_amd64.exe salus.exe
```
Then add the directory containing `salus.exe` to your `PATH`.

### Verifying build provenance (optional)

Releases after 1.0.1 include a signed build provenance attestation for every
archive. Verify an archive with GitHub CLI 2.97 or newer (older versions have
a verification bypass, GHSA-mm27-mwq9-fr5g), replacing `vX.Y.Z` with the tag
of the release you downloaded:

```bash
gh attestation verify salus_linux_amd64.tar.gz --repo jabbott-iii/Salus \
  --signer-workflow jabbott-iii/Salus/.github/workflows/cd.yml \
  --source-ref refs/tags/vX.Y.Z
```

This checks that the archive was built by this repository's release workflow
(`cd.yml`) from that tag. Verification fails for a modified archive, and for
archives from manual CD runs, which are built from a branch rather than a
release tag. The 1.0.0 and 1.0.1 archives have no build provenance
attestation.

### Building from source

```bash
git clone https://github.com/jabbott-iii/Salus.git
cd Salus
make build
./salus --version
```

`make build` runs `CGO_ENABLED=1 go build -o salus .`; run that command
directly if Make is not installed. A local build reports `salus version dev`.
Release builds set the version with `-ldflags "-X main.version=vX.Y.Z"`.

## Usage

### Quick start

```bash
salus check list    # list the available checks
salus check run     # run every check and record the run as a job
salus jobs list     # list recorded runs
```

Example output (values depend on the host):
```text
$ salus check run --only disk-space,memory,cpu-load,misconfig
Environment Health Check

[PASS] disk-space        /: 46.1% used (53.9% free)
[PASS] memory            memory 39.1% used, swap 8.9% used
[PASS] cpu-load          load average 1.80 across 16 CPU(s) (11% per-core)
[PASS] misconfig         no common misconfigurations detected
```

### Commands

Salus is organized into focused command groups:

- `salus check` — list and run health checks
- `salus jobs` — view past health check runs
- `salus --version` — print the Salus version

#### check

- `salus check list` — list available health checks
- `salus check run` — run health checks and report the results

Examples:
```bash
salus check list
salus check run
salus check run --only disk-space,memory,cpu-load
salus check run --service nginx
salus check run --json
salus check run --fail-only
salus check run --disk-path /data
salus check run --disk-warn 70 --disk-fail 85 --timeout 10s
```

Flags for `check run`:
- `--only` — comma-separated list of checks to run (default: all)
- `--service` — systemd service name to check uptime for (defaults to host uptime)
- `--disk-path` — mount path to check for free disk space (default `/`)
- `--disk-warn`, `--disk-fail` — disk usage percent at which `disk-space`
  reports WARN and FAIL (defaults `80` and `90`)
- `--mem-warn`, `--mem-fail` — memory usage percent at which `memory` reports
  WARN and FAIL (defaults `80` and `90`)
- `--load-warn`, `--load-fail` — 1-minute load average per CPU, in percent, at
  which `cpu-load` reports WARN and FAIL (defaults `80` and `100`; `100` means
  a load average equal to the number of CPUs)
- `--timeout` — time limit for each `docker`, `kubectl`, or `systemctl`
  command (default `3s`)
- `--json` — output results as JSON
- `--fail-only` — only show WARN and FAIL results in text output
- `--quiet` — suppress report output, including `--json` (still sets the exit code)
- `--no-save` — do not persist this run to the database

Threshold values must be numbers greater than `0`, each WARN value must be
lower than its FAIL value, and disk and memory values cannot exceed `100`. If
you set a FAIL value below the default WARN value, lower the WARN value too
(for example `--disk-warn 60 --disk-fail 75`). `--timeout` takes a duration
such as `500ms`, `10s`, or `1m` and must be positive. Salus rejects an invalid
value before running any check.

With `--json`, `check run` prints an array with one object per check;
`duration_ns` is the check's run time in nanoseconds:
```json
[
  {
    "key": "misconfig",
    "status": "PASS",
    "message": "no common misconfigurations detected",
    "duration_ns": 20253
  }
]
```

#### jobs

- `salus jobs list` — list recent health check runs
- `salus jobs show [job-id]` — show details for a specific run

Examples:
```bash
salus jobs list
salus jobs list --limit 50
salus jobs show 7
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Every check passed, or the command succeeded |
| `1` | At least one check reported WARN, and none reported FAIL |
| `2` | At least one check reported FAIL |
| `3` | Salus could not complete the command |

`check run` uses codes `0` to `2`, which makes it suitable for scripts and CI
pipelines. Any command exits with code `3` for invalid flags, arguments, or
threshold values (including an unknown command or subcommand), an unknown
check name in `--only`, a job that does not exist, or a database error.
Errors are written to stderr, so stdout carries only command output (for
example, clean JSON with `--json`).

## Configuration

Salus stores job history in a SQLite database. The database is created on the
first command that needs it (`check run` without `--no-save`, `check list`,
`jobs list`, `jobs show`); `--help`, `--version`, and `check run --no-save`
never create it.

| Setting | Default | Purpose |
|---|---|---|
| `SALUS_DB_PATH` (optional) | Per-user location below | Path of the database file. Overrides the default. |

Default database location when `SALUS_DB_PATH` is not set:

| Platform | Path |
|---|---|
| Linux and other Unix | `$XDG_DATA_HOME/salus/salus.db`, or `~/.local/share/salus/salus.db` when `XDG_DATA_HOME` is unset |
| macOS | `~/Library/Application Support/salus/salus.db` |
| Windows | `%LOCALAPPDATA%\salus\salus.db` |

Salus creates a new database file with mode `0600` and any missing parent
directories with mode `0700`. The `misconfig` check warns if the database file
is readable or writable by group or other users (on Linux and macOS).

Check thresholds and the command timeout are set for each run with the
`check run` flags above. There is no configuration file.

### Upgrading from 1.0.0

- **Database location:** 1.0.0 created `salus.db` in the current working
  directory. Since 1.0.1, Salus no longer reads that file by default. To keep
  your job history, move it to the location above, or set
  `SALUS_DB_PATH=/path/to/salus.db`.
- **Database permissions:** databases created by 1.0.0 are usually readable by
  other users (mode `0644`), which now makes `misconfig` report `WARN` (exit
  code `1`). Restrict the file with `chmod 600 /path/to/salus.db`.
- **`--service` values** must be systemd unit names (letters, digits, and
  `:-_.\@`, not starting with `-`). Other values fail the `service-uptime`
  check without running `systemctl`.
- **Docker volumes:** the 1.0.0 image ran as root, so a database it created in
  the `/app/data` volume is owned by root. Since 1.0.1 the image runs as UID
  10001, so hand the existing data over once:

  ```bash
  docker run --rm --user 0 --entrypoint sh -v salus-data:/app/data salus \
    -c 'chown -R 10001:10001 /app/data && chmod 700 /app/data && find /app/data -type f -exec chmod 600 {} +'
  ```

## Containerization

The repository includes a multi-stage `Dockerfile`. Build the image:
```bash
docker build -t salus .
```

Run a health check:
```bash
docker run -it --rm \
  -v salus-data:/app/data \
  -e SALUS_DB_PATH=/app/data/salus.db \
  salus check run
```

Note:
 - The container runs as the unprivileged user `salus` (UID and GID `10001`)
   and uses `SALUS_DB_PATH=/app/data/salus.db` by default.
 - Database state is persisted in `/app/data`. A new named volume, as in the
   example above, is writable by the container automatically. For a bind
   mount, make the host directory writable by UID `10001` first (for example
   `sudo chown 10001:10001 /path/to/data`). A volume created by Salus 1.0.0
   needs a one-time ownership fix (see
   [Upgrading from 1.0.0](#upgrading-from-100)).
 - The `docker-status` and `kubernetes-status` checks are not supported inside
   the container. The image does not include the `docker` or `kubectl` CLIs,
   so those checks report `WARN` (`... CLI not found in PATH`). Run the
   `salus` binary on the host for them. Do not mount the Docker socket into
   the container: it gives the container root-equivalent control of the host.
 - Resource checks inside a container see the container's view: `/proc`
   memory and load figures are host-wide, and `disk-space` measures the
   container filesystem unless you mount a host path and pass `--disk-path`.

## Testing and quality checks

Run these from the repository root. Each `make` target wraps the command shown.

| Command | Runs |
|---|---|
| `make test` | `go test ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run ./...` three times, with `GOOS=linux`, `GOOS=darwin`, and `GOOS=windows` |
| `make fmt` | `gofmt -s -w .` |
| `make cover` | `go test -coverprofile=coverage.out ./...`, then `go tool cover -func=coverage.out` |
| `make build` | `CGO_ENABLED=1 go build -o salus .` |

`make lint` expects golangci-lint on `PATH`; point it at another binary with
`make lint GOLANGCI_LINT=/path/to/golangci-lint`. Also run
`go test -race ./...` for concurrency-related changes, and
`go mod tidy && git diff --exit-code`, because CI fails when `go.mod` or
`go.sum` drift.

CI runs `go vet`, golangci-lint, and the tests on Linux, macOS, and Windows,
and smoke-tests the built binary. The Security workflow runs CodeQL, gosec, and
govulncheck. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full checklist
before opening a pull request.

## Project structure

```text
.
├── main.go, version.go   Entry point: runs the CLI, maps results to exit codes, --version
├── internal/             All application code (one Go package)
│   ├── logic-cli.go      Cobra commands and flags
│   ├── health*.go        Check registry, thresholds, and the individual checks
│   ├── report.go         Text and JSON output, exit codes
│   ├── database*.go      Database location, file permissions, GORM models
│   ├── scan-store.go     Job and result storage
│   └── seed.go           Built-in check catalog
├── .github/              CI, security scanning, Docker smoke tests, releases, Dependabot
├── Dockerfile            Multi-stage container image (runs as UID 10001)
├── Makefile              Development targets and release tagging
├── intel/                Maintainer documents: architecture, security, plans
├── CONTRIBUTING.md       Contribution workflow and validation
└── LICENSE, NOTICE       Apache-2.0 license and third-party notices
```

## Contributing and license

Contributions start with an issue; see [CONTRIBUTING.md](CONTRIBUTING.md).
Salus is licensed under the Apache License 2.0 (see [LICENSE](LICENSE)), and
third-party notices are in [NOTICE](NOTICE).
