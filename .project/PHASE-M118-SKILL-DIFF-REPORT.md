# Phase Report — Milestone M118 (`agt skill diff`)

> Status: **shipped** · Date: 2026-06-02 · skill management.

## Why

`agt skill history` shows a skill's lifecycle *events* (promote/quarantine/revert)
but not how its *body* changed between versions. Before promoting a reverted
skill, or auditing what the Forge rewrote, an operator had to eyeball two bodies
by hand. `agt skill diff` shows the line-level change.

## What shipped

- **`agt skill diff <id> [<id2>]`** — one id diffs the skill against its lineage
  parent (the most recent ancestor) — "how did this evolve?"; two ids diff the
  first (old) against the second (new). Reuses `CmdSkillGet` (which already
  returns the body + lineage) — no new control-plane command. Exit 3 when a
  skill/parent is absent, 2 on usage.
- **`lineDiff`** — a pure LCS line diff emitting unified-style ` `/`-`/`+` ops,
  with an `N added, M removed` summary. Deterministic and unit-tested.

## Design notes

- **Lineage-aware default.** With one id, the parent is taken from the skill's
  `lineage` tail, so the common "what changed in this revision?" needs no second
  argument.
- **No new RPC.** Skill bodies are already on `CmdSkillGet`; the diff is pure
  client-side.

## Tests

- `TestLineDiff` — identical → all context; a changed middle line → `-old`/`+new`
  with surrounding context; pure add; pure remove; empty.
- `TestSplitLines` — empty → nil; CRLF normalised.
- `TestSkillGet_ReturnsBodyForDiff` (control plane) — two Forge-seeded skills
  return distinct bodies via `CmdSkillGet` (the diff's data path).

Test count: **1391 → 1394**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Note on live demo

Skills are created by the Forge (LLM-assisted, from task transcripts) — there is
deliberately no `agt skill create`. So a fully-live `agt skill diff` against a
running daemon needs forged skills; the diff math is unit-proven and the
body-retrieval path is integration-tested with Forge-seeded skills instead.

## Example output

```
$ agt skill diff <new-id>          # vs its parent
--- <parent-id>
+++ <new-id>
  line one
- line two
+ line TWO
+ line three

2 added, 1 removed
```
