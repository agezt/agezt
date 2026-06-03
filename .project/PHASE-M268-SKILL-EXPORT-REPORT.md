# M268 — `agt skill export`: portable, verifiable skill bundles

## Why
The remaining big frontier is a **skill/plugin marketplace** (discover + install
shareable skills). Before any registry can exist, a skill needs a **portable
artifact** — exactly the foundation the journal export (M101) and backup (M113)
arcs established for their domains. Skills had a full lifecycle (list / show /
history / promote / quarantine / revert / diff) but no way to move one between
Agezt instances. This milestone adds the export half, chosen as the concrete,
offline-testable, low-risk first slice of the marketplace arc.

A skill's id is content-addressed — `hex(BLAKE3("skill" \0 name \0 body))` — so a
bundle can be made **self-verifying**: recomputing the address from the bundle's
name + body and checking it against the claimed id detects any tampering, with no
signature or registry needed (the same property the journal bundle uses).

## What
- **`cmd/agt/skill_export.go`** — new `agt skill export <id> [--out <file>]`:
  - `skillBundle` / `skillBundleBody` — the portable format: a small manifest
    (tool, format version, export timestamp) + the skill's **content fields
    only** (id, name, description, triggers, body, tools_required, version,
    lineage). Instance-local state (status, metrics, source_event, timestamps) is
    excluded *by construction* — `skillBundleBody` simply does not declare those
    fields, so the JSON round-trip drops them.
  - `buildSkillBundle(skillMap, nowMS)` — projects the daemon's `CmdSkillGet`
    skill map into a bundle (pure, testable).
  - `verifySkillBundle(b)` — recomputes `skill.ContentID(name, body)` and checks
    it equals the bundle's id; also rejects an empty name/id (pure, testable).
  - `cmdSkillExport` — dials the daemon, fetches the skill (exit 3 if absent like
    `skill show`), builds + verifies the bundle (refuses to emit a skill that
    fails its own content address), and writes it to stdout or `--out <file>`.
  - Wired into the `skill` dispatch + help; the subcommand list now reads
    `…|diff|export`.

## Files
- `cmd/agt/skill_export.go` — command + bundle format + pure helpers (new).
- `cmd/agt/skill.go` — `export` dispatch, help line, subcommand lists (edited).
- `cmd/agt/skill_export_test.go` — 2 tests (new): a built bundle keeps the
  content fields, verifies against its address, and leaks no instance state
  (status/metrics/source_event/timestamps absent from the JSON); a tampered body
  fails verification, and a name-less/id-less bundle is rejected.

## Verification
- `go test ./cmd/agt/ -run 'TestBuildSkillBundle|TestVerifySkillBundle'` — green;
  full suite **1858 → 1860** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven** end-to-end: seeded a content-addressed skill into a temp home,
  ran a mock daemon, `agt skill list` showed it, `agt skill export <id>` emitted
  a clean bundle to stdout (content fields only), and `--out` wrote the file with
  a notice. A grep confirmed the exported file carries **no** status / metrics /
  created_ms / source_event.

## Scope notes
- Pure read-side + a new portable format; no new event, no daemon change (reuses
  `CmdSkillGet`).
- Sets up the marketplace arc's next slice: **`agt skill import <bundle>`** —
  verify the bundle (reuse `verifySkillBundle`) then install it as a fresh draft
  in the local skill store (needs a control-plane "skill put" path, mirroring
  `agt journal import`). A registry/discovery layer builds on top of the bundle.
