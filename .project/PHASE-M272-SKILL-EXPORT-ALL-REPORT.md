# M272 — `agt skill export --all`: publish a whole skill library

## Why
The skill marketplace could discover (`registry`) and install (`import` /
`registry --install`) from a directory of bundles, but populating that directory
meant running `agt skill export <id>` once per skill. This milestone adds the
publisher's bulk step: export every skill into a directory in one command,
producing exactly the directory registry the consumer side reads.

## What
- **`cmd/agt/skill_export.go`** — `agt skill export --all [--dir <dir>]`:
  - `exportAllSkills(dir)` makes a single `CmdSkillList` call (whose records
    already include bodies), builds + verifies a bundle per skill, and writes one
    file per skill into `dir` (default `.`). Each file is the same self-verifying
    bundle `agt skill export <id>` produces. Reports the count and a `agt skill
    registry <dir>` browse hint; a skill that fails to build/verify is skipped
    with a warning and the command exits non-zero.
  - `safeSkillFilename(name, id)` — a filesystem-safe filename: lowercased name
    with non-alphanumeric runs collapsed to a dash, plus a 12-char short id so
    two versions of the same name never collide. Empty/odd names fall back to
    `skill-<id>`.
  - `cmdSkillExport` gained `--all` / `--dir`; `--all` with a positional id is a
    usage error (reported before any dial).

## Files
- `cmd/agt/skill_export.go` — `exportAllSkills`, `safeSkillFilename`, flag
  handling (edited).
- `cmd/agt/skill.go` — help line for the `--all` form (edited).
- `cmd/agt/skill_export_test.go` — 2 tests (new): `safeSkillFilename` slugifies
  (spaces/`@`/underscore/trim/all-symbol) and is collision-safe across ids;
  `--all` with an id is a usage error before dialing.

## Verification
- `go test ./cmd/agt/ -run 'TestSafeSkillFilename|TestCmdSkillExport_All'` —
  green; full suite **1871 → 1873** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven**: seeded 3 skills (names with a space, an `@`, and an
  underscore; mixed draft/active), ran `agt skill export --all --dir <dir>` →
  3 files with slugified names (`diagnose-ci-failures-…`, `rollback-v2-…`,
  `deploy_service-…`), then `agt skill registry <dir>` listed all three `[ok]`
  with their original names + descriptions.

## Scope notes
- Closes the publisher→consumer loop fully: **export --all (publish) → share the
  directory → registry (discover) / registry --install (install)**, all on the
  verifiable content-addressed bundle format.
- Exports the whole library regardless of lifecycle state (drafts included) —
  imported skills become drafts on the target anyway, so publishing a draft is
  harmless and lossless.
