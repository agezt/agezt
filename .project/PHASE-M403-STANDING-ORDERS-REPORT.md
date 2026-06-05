# M403 — Chronos standing orders: model + store + management surface (SPEC-16 §4)

## Context
SPEC-16 §4 defines the standing-order DSL — a persistent goal kept alive by
Chronos + Pulse: named, pausable, with cron/event triggers, observers, a scope,
an initiative ceiling, and a briefing. It was the last untouched offline-doable
LARGE feature. The spec sketches it as Flow-Studio-authored YAML, but Agezt is
stdlib-first (no YAML dependency, DECISIONS B0c), so the on-disk and wire form is
JSON — the same declarative shape. This milestone is the **management surface**
(model + store + journaled CRUD + `agt standing`); the runner that fires triggers
and drives observe→salience→initiative→briefing is layered on next.

## What
- **`kernel/standing`** (new package): `Order` (id, name, enabled, triggers,
  observers, scope, initiative{mode,max_trust,budget}, briefing, plan,
  timestamps), `Trigger` (cron|event), `Initiative` modes; pure `Validate`
  (name + ≥1 well-formed trigger + known mode); file-backed `Store`
  (atomic JSON, mutex-guarded) mirroring `kernel/cadence.Store`:
  Add (ULID + timestamps), List (deterministic), Get, SetEnabled, Remove, Count.
- **`kernel/event/kinds.go`** — `standing.created` / `standing.updated` /
  `standing.removed`.
- **`kernel/runtime`** — kernel opens the store under `<base>/standing`,
  `Standing()` accessor + journaling wrappers `AddStanding` / `SetStandingEnabled`
  / `RemoveStanding` (publish the lifecycle events).
- **`kernel/controlplane`** — `standing_list|add|set_enabled|remove` commands +
  handlers.
- **`cmd/agt/standing.go`** — `agt standing add|list|pause|resume|remove`.

## Verification
- **`kernel/standing/standing_test.go`**: `Validate` matrix (8 cases);
  Add/List/Get/Count; pause/resume/remove (+ `ErrNotFound`); persistence across
  reopen.
- **`kernel/controlplane/standing_test.go`** `TestStanding_CRUDRoundTrip`:
  add → list → pause → remove over the control plane, invalid-order rejected, and
  all three lifecycle events journaled.
- **`cmd/agt/standing_test.go`**: `renderStandingLine` (on/off, trigger count,
  mode), add requires name + a trigger, unknown subcommand, pause requires id.
- **Negative control:** removing the `standing.created` publish in `AddStanding`
  → the control-plane round-trip test FAILs ("no standing.created event
  journaled"); restored byte-identical.
- **Live demo** (mock): `agt standing add --name "portfolio watch" --cron "0 8 * *
  *" --event "github.>" --plan … --mode act_or_ask --max-trust L2` →
  `agt standing list` shows `[on] portfolio watch · 2 trigger(s) · act_or_ask`;
  pause → `[off]`, resume, remove → "no standing orders"; the journal carries
  `standing.created`×1, `standing.updated`×2 (pause+resume), `standing.removed`×1.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2245** passing (was 2228; +17). CHANGELOG (Added, user-visible command).

## Scope notes
- This is the foundation/management slice of the Chronos arc. **Next: the runner**
  — fire cron triggers (reuse `kernel/cadence`) and event triggers (bus monitor on
  the order's subject) to launch the order's plan as a run, bounded by its
  initiative ceiling/budget, then brief. Plus web surfacing and `agt standing why`.
