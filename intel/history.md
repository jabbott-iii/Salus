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
