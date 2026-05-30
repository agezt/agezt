# Phase Report — Milestone M9 (Reflection v1)

> Status: **shipped** · Date: 2026-05-30
> Phase 2 (ROADMAP product-M2 "Memory & self-improvement"), slice 3 of 3
> — the **final** slice. Closes the learning loop: the system reviews its
> own behaviour from the journal and recalibrates (SPEC-05 §6). With this,
> **Phase 2 is complete** — the system remembers, models its world, learns
> skills, and now tunes itself.

## Scope

Forge (M8) improves *skills*; Reflection improves *judgment*. On demand
(`agt reflect run`) or on a timer, a pass folds the journal into
observations, applies the one safe auto-adjustment, surfaces advisory
proposals, and journals a report.

**The SPEC fixes the safe/propose split (§6.4) — no design fork:**
- **Auto-applied (safe bounds):** **world-model decay** (§6.3) — entities
  not referenced within a staleness window lose weight (floored, never
  dropped); recently-active ones hold. Owned data, reversible, touches no
  autonomy.
- **Proposed only (never silently applied):** anything affecting
  judgment/autonomy — Pulse cadence, autonomy on denied tools, failing
  runs. v1 surfaces these as **advisory proposals** via deterministic
  rules over the folded counts. Reflection may only ever *lower* autonomy,
  never raise (DECISIONS F3) — and v1 only *proposes* it.

## What shipped

### New `kernel/reflect`
A read-mostly **engine** (no store of its own — a report is derived from
the journal and journaled back, like `agt runs list`/`skill history`):
- `Reflect(ctx, corr)` — fold the journal → `Observations` (tasks
  started/completed/failed, briefs, skill activations, approvals
  granted/denied, world entities) → apply `world.Decay` → derive
  `Proposals` via pure rules → publish `reflection.completed`.
- `Latest()` — read the newest report back from the journal.
- `proposals(obs)` — deterministic, LLM-free rules (pulse volume,
  approval-denial excess, run-failure rate). Each fires at a configurable
  threshold and stays silent below it.

### `kernel/worldmodel` — `Graph.Decay`
`decay.go`: iterate active entities; those stale beyond `StaleAfter` have
`weight *= Factor` (floored at `Floor`), journaled as a
`worldmodel.entity.upserted` with `action:"decay"` (decay is just a
downward reinforce — **no new event kind**). Only ever lowers weight,
never tombstones; returns the count decayed.

### Event kinds (append-only)
`reflection.completed` — added to the const block and `knownKinds`.

### Runtime / control plane / CLI / daemon
- `runtime.go` — the reflect engine is constructed in `Open` (it only
  needs the journal + world + bus, already held); `Reflect()` accessor.
  No per-run injection or config knob (reflection is *invoked*, not woven
  into every run) — keeps the change minimal.
- `kernel/controlplane/reflect.go` — `CmdReflectRun` (mints a correlation,
  runs a pass, returns the report) / `CmdReflectShow` (latest).
- `cmd/agt/reflect.go` — `agt reflect run|show [--json]`.
- `cmd/agezt/main.go` — optional **periodic trigger**: `AGEZT_REFLECT_EVERY`
  (e.g. `24h`) starts a ticker goroutine on the daemon ctx (mirrors
  Pulse); absent → on-demand only. Banner line.

| Env var | Meaning |
|---|---|
| `AGEZT_REFLECT_EVERY` | duration (e.g. `24h`) to run a reflection pass on a timer; unset = on-demand only |

## Design rules followed

- **No new external dependency** — stdlib only; `go.mod` unchanged
  (POLICY).
- **No new store** — a report is a journal fold + a journaled event, the
  pattern `runs`/`inbox`/`skill history` already use. Reflection adds the
  least surface of any slice.
- **Reuses the world graph for decay** — one new `Graph.Decay` method that
  journals through the existing `publish`, under the reflection's
  correlation, so `agt why` links a weight drop to the pass that caused it.
- **Safe-by-default** — decay only lowers weight and floors it;
  autonomy/judgment changes are *proposed*, never applied; reflection can
  only ever reduce autonomy, per DECISIONS F3.

## Test coverage

~20 new tests; `go test ./...` green on host (windows) + `GOOS=linux`
cross-compile; `go vet` clean. Package count 45 → 46 (added
`kernel/reflect`).

- `kernel/reflect`: fold produces correct counts from a seeded journal;
  the pass invokes decay + journals `reflection.completed`; proposal rules
  fire at thresholds and stay silent below; `Latest` returns the newest.
- `kernel/worldmodel`: `Decay` lowers stale weights to the floor, leaves
  fresh ones untouched, journals + returns the count, is idempotent at the
  floor, and uses safe defaults.
- `kernel/runtime`: reflection runs through the kernel, observes a real
  run, journals, and `Latest` reads it back.
- `kernel/controlplane` (real TCP pair): run→show round-trip; show-before-
  any-pass is not-found.
- `cmd/agt`: reflect help / arg-validation exit codes.

### Manual end-to-end (mock provider)
The daemon banner reports the reflection mode. After two runs and a world
entity, `agt reflect run` printed a report — counts (`2 started, 1
completed, 1 failed`), `0 stale entities decayed` (the entity was fresh),
and an advisory `[tasks]` proposal — and journaled `reflection.completed`;
`agt reflect show` re-printed it from the journal. The decay arithmetic
(stale → floored weight, journaled under the pass correlation) is proven
by `worldmodel/decay_test` and exercised live by the engine; a live decay
*observation* needs a 14-day-stale entity, so it's covered by tests rather
than the quick demo.

## Deferred (named for later)

- **LLM-assisted reflection narrative** (v1 is deterministic/offline).
- **Auto-tuning salience cadence/thresholds** — needs explicit dismissal-
  feedback events first (snooze/dismiss/delete), which don't exist yet.
- **Trust-ladder change proposals wired to the approval flow**,
  predictions-vs-reality scoring, cost/efficiency analysis.
- **Scheduled-by-default cadence** (v1 is on-demand + optional
  `AGEZT_REFLECT_EVERY`).

## Closes / next

**Phase 2 (product-M2 "the system starts learning") is complete.** The
six in-process residents — agent loop, memory, world model, Forge,
reflection, pulse — plus channels and the operator CLI give Agezt a full
cognitive loop: it remembers facts, models the operator's world, learns
and governs skills, proactively notices change, and reviews and tunes its
own behaviour — all journaled, content-addressed, and reversible.

Next per ROADMAP M-series: **Phase 5 — Web UI** (Flow Studio, Live
Monitor, Memory/World/Skill Explorer, the Web Inbox) — a surface over the
control plane + journal everything here already exposes — or hardening
toward a tagged release.
