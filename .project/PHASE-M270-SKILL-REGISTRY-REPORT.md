# M270 — `agt skill registry`: discover skill bundles in a directory

## Why
M268/M269 made a skill portable (export) and installable (import). The missing
piece before this is a marketplace is **discovery**: seeing what is available to
install. The simplest, offline-testable registry is a **directory of bundles** —
no remote index, no new protocol. This milestone lists the verifiable bundles in
a directory so an operator can browse what is on offer and copy the exact import
command, with tampered/malformed bundles surfaced up front.

## What
- **`cmd/agt/skill_registry.go`** — new `agt skill registry <dir> [--json]`:
  - `scanSkillRegistry(dir)` globs `*.skill.json`, parses each as a bundle, and
    builds a `registryEntry` (path, name, version, id, description, triggers,
    `Verified`, `Err`). A file that does not parse as a bundle (or has no skill
    name) is reported with `Err` set — surfaced, not dropped. `Verified` reuses
    M268's content-address check, so a tampered bundle is visible at a glance.
    Entries are sorted by name then path. Pure and unit-testable.
  - `cmdSkillRegistry` renders the list (name / version / short id / `[ok]` or
    `[TAMPERED]`, description, and the ready-to-run `agt skill import <path>`),
    flags malformed entries with `(!)`, and exits non-zero if any bundle is bad
    so a registry with a tampered file is caught. `--json` emits the entries.
  - Wired into the `skill` dispatch + help; subcommand list now ends `…|registry`.

## Files
- `cmd/agt/skill_registry.go` — command + `scanSkillRegistry` + `registryEntry`
  (new).
- `cmd/agt/skill.go` — `registry` dispatch, help line, subcommand lists (edited).
- `cmd/agt/skill_registry_test.go` — 3 tests (new): scan classifies
  valid/tampered/junk and sorts by name; an empty dir yields no entries; the
  command is a usage error with no dir and exits non-zero on a tampered registry.

## Verification
- `go test ./cmd/agt/ -run 'TestScanSkillRegistry|TestCmdSkillRegistry'` — green;
  full suite **1864 → 1867** (+3), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven**: exported a real bundle into a dir, added a byte-flipped copy
  and a non-JSON file, then `agt skill registry <dir>` listed all three — the
  valid one `[ok]` with its import command, the altered one `[TAMPERED]`, the
  non-JSON one with `(!)` and its parse error — exiting 1 with a clear notice.

## Scope notes
- Pure offline file read; composes with M269 — browse a directory registry, then
  `agt skill import <path>` the one you want (the import re-verifies before
  installing, so discovery and install are independently safe).
- Completes a usable local marketplace loop: **export (M268) → share a directory →
  registry (M270) → import (M269)**. A remote/HTTP registry, if ever wanted, can
  reuse `scanSkillRegistry`'s entry shape over a fetched index.
