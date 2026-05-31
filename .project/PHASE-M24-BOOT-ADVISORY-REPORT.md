# Phase Report — Milestone M24 (Boot-time model advisory)

> Status: **shipped** · Date: 2026-05-31
> SPEC-15 §1. The direct follow-up named in M23: surface the agent-readiness
> capability check in the daemon's startup banner, so an operator learns about a
> tool-incapable primary model at boot rather than mid-run.

## Why

M23 added `agt provider check --caps`, a network-free capability view that warns
when a model doesn't advertise tool-use — the prerequisite for a tool-driven
agent loop. But that's a command an operator has to *think to run*. The moment
they'd most want the warning is when they actually start the daemon on a given
model. M24 closes that last step: the same advisory now appears in the boot
banner, automatically, for the auto-selected primary model.

## What shipped

- **`modelAdvisory(cat, model)` (`cmd/agezt/main.go`)** — resolves the selected
  primary model against the catalog (`FindModel`, which handles both bare and
  `provider/model` ids) and returns its `catalog.Model.AgentWarnings` joined into
  one line, or `""`. Reuses the exact M23 helper, so the banner and `agt provider
  check --caps` can never disagree.
- **Banner wiring** — one new line printed directly under `governor` when the
  advisory is non-empty:
  `  model advisory   : ⚠ model "X" does not advertise tool-use …`
- **Conservative by construction** — a model the catalog doesn't know (the
  offline mock, a bare local model, an unsynced id) returns no advisory, so the
  feature informs without crying wolf. Non-blocking: it never fails the boot.

No `go.mod` change, no new event kind, no daemon hot-path change — a single
catalog lookup at startup.

## Proven

- **Unit (`cmd/agezt`):** `modelAdvisory` over a synthetic catalog — a
  `tool_call=false` model yields a tool-use advisory; a tool-capable model yields
  `""`; the offline mock, an unknown id, the empty string, and a nil catalog all
  yield `""` (no false alarm).
- **Live (network-free, custom catalog):** booting `AGEZT_PROVIDER=acme
  AGEZT_MODEL=acme-mini` (a `tool_call=false` model) prints
  `model advisory   : ⚠ model "acme-mini" does not advertise tool-use …`
  directly under the governor line; booting the same provider on `acme-pro`
  (`tool_call=true`) prints no advisory line. Both boot cleanly.

1 new test; suite **1191** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc

M23 made model capabilities inspectable on demand; M24 makes the headline gap
impossible to miss — it's on screen every boot. Together they turn the catalog's
long-inert capability data into an operator-facing safety signal at the two
moments that matter: when choosing a model (`--caps`) and when running on it
(banner).

## Deferred — named

- **Request-time capability validation** in the Governor (reject a tools request
  to a non-tool model with a clear pre-flight error, or down-route to a
  tool-capable fallback) — the stronger enforcement step, still held for after
  the advisory path proves out against real catalog-data variance.
- **Per-task-route advisories** — `AGEZT_TASK_MODEL_OVERRIDES` can route a task
  type to a different model; the banner only checks the primary today.
