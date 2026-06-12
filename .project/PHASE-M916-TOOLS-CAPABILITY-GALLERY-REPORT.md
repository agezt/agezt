# Phase M916 — Searchable capability gallery in the Tools view

## Ask
Continuation of the owner's "make the webui much more visual / see-everything"
direction. The Tools view's **"Available tools"** list was a flat 2-column grid
with only a used/idle badge — hard to scan once the agent has built-ins + MCP +
forged + skill tools, and it didn't show what each tool *is allowed to do* or how
it's been used.

## What shipped — `frontend/src/views/Tools.tsx`
The "Available tools" section became a **searchable, capability-grouped gallery**.
The catalog endpoint already returns each tool's **Edict `capability`** — now
surfaced:
- **Search box** — filter by name / description / capability (case-insensitive).
- **Capability filter chips** — one per distinct capability (with counts), plus
  "all"; click to narrow. The security axis the owner cares about
  ([[default-allow-posture]]) is now a first-class browse dimension.
- **Richer cards** — each tool shows its name, a **source badge** (mcp / forged /
  skill, inferred from the name prefix), its **capability** (shield), inline
  **usage** (call count or "idle", error count, avg latency — folded from the
  per-tool stats), and the description. Used tools sort first by call volume, then
  idle alphabetically.

The "Usage by tool" bars and the live "Invocation log" are unchanged.

## Pure helpers (exported, unit-tested)
- `toolSource(name)` — mcp / forged / skill / builtin from the name prefix.
- `mergeToolViews(catalog, byTool)` — join catalog + usage, sorted used-first.
- `filterTools(views, query, capability)` — search + capability filter.
- `capabilityCounts(views)` — per-capability tallies for the chips.

## Tests — `frontend/src/views/Tools.test.tsx`
Unit tests for all four helpers (source inference, merge+sort, search/capability
filtering, capability tallies), plus the existing render tests updated for the new
call-count badge. 8 tests in the file; full suite **547 pass**.

## Gate
`tsc` ✓ · full vitest **547 pass** (80 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only (the `capability` field was already in the `/api/tools_catalog`
payload).

## Process
Built in an isolated git worktree (`AGEZT-tools`, branch
`feat/m916-tools-gallery`) from `origin/main`. Numbering: M915 was taken by the
concurrent session's per-agent-memory work (PR #342) mid-flight, so this renumbered
to **M916** — re-checked `git log origin/main` + open PRs right before committing.
