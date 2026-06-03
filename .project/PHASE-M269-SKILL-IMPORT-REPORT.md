# M269 — `agt skill import`: install a skill from a portable bundle

## Why
M268 added `agt skill export` (a portable, content-addressed, self-verifying
skill bundle). This milestone adds the read-back half — installing a bundle into
another Agezt instance — completing the export/import pair that any marketplace
needs. It mirrors the journal export→import (M101→M102) and backup→restore (M113)
pairs the project already established.

## What
- **`kernel/controlplane/protocol.go`** — new `CmdSkillImport = "skill_import"`
  (args: name, body required; description, triggers, tools_required optional;
  returns `{id, name, status, created}`).
- **`kernel/controlplane/skill.go`** — `handleSkillImport` routes through
  `Forge().Create`, so the imported skill is content-addressed, deduped against
  an identical existing skill, and journaled (`skill.created`). It arrives as a
  fresh **draft** regardless of the source's lifecycle — never auto-active — so an
  operator must still `promote` it before it injects into runs. Reuses the
  existing `stringArg` / `argStringList` arg helpers.
- **`kernel/controlplane/server.go`** — dispatch `case CmdSkillImport`.
- **`cmd/agt/skill_import.go`** — `cmdSkillImport`: reads the bundle, parses it,
  and **verifies its content address OFFLINE** (`verifySkillBundle` from M268)
  before dialing — a tampered bundle never reaches the daemon. Then sends
  `CmdSkillImport`, cross-checks that the daemon's re-derived id matches the
  bundle's claimed id, and reports installed-vs-deduped with a `promote` hint.
- **`cmd/agt/skill.go`** — `import` dispatch, help line, subcommand lists.

## Files
- `kernel/controlplane/protocol.go`, `kernel/controlplane/skill.go`,
  `kernel/controlplane/server.go` — command + handler + dispatch (edited).
- `cmd/agt/skill_import.go` — CLI command (new).
- `cmd/agt/skill.go` — wiring (edited).
- `kernel/controlplane/skill_test.go` — 2 tests (new): import installs a fresh
  non-active draft at the right content address and re-import dedupes
  (created=false); import without a body errors.
- `cmd/agt/skill_import_test.go` — 2 tests (new): a tampered bundle is rejected
  offline (no daemon dialed) with a content-address-mismatch error; a missing
  bundle path is a usage error.

## Verification
- `go test ./cmd/agt/ ./kernel/controlplane/ -run TestSkillImport` — all green;
  full suite **1860 → 1864** (+4), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven** across two homes: seeded an active skill in home A, `agt skill
  export … --out diag.skill.json`, then in a fresh home B `agt skill import` →
  "installed as a new draft" (`status: draft`, `0 active` in `skill list`);
  re-import → "already present (refreshed)" (content-address dedupe); a
  byte-flipped body → "bundle INVALID: content-address mismatch" (rejected
  offline, before the daemon).

## Scope notes
- The export/import pair is now complete: a skill authored or evolved on one node
  can be moved to another, verifiably and as a fresh draft.
- Next marketplace slice: a discovery/registry layer (list available bundles from
  a source, fetch + `import`), building on this portable, self-verifying format.
- Imported skills are intentionally drafts — installing a bundle can never put an
  unreviewed skill into the live retrieval pool; promotion stays a deliberate
  operator action.
