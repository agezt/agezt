# Phase Report — Milestone M27 (Capability matrix)

> Status: **shipped** · Date: 2026-05-31
> SPEC-15 §1. The remaining M23 item: `agt provider check --caps --all` — a
> side-by-side capability table so an operator picks a model by capability
> instead of inspecting one at a time.

## Why

M23 added `--caps` for a single model; M24–M26 surfaced the headline gap at boot,
in the Governor, and in `agt doctor`. The one ergonomic gap left was comparison:
"across the providers I have, which models can do tools / vision / reasoning, and
how big is their context?" Answering that meant running `--caps` once per
provider and eyeballing. M27 makes it one command.

## What shipped

- **`agt provider check --caps --all` (`cmd/agt/check.go`)** — a capability matrix:
  one row per *supported* catalog provider (its selected model, `$AGEZT_MODEL`
  when it serves that id, else the provider's first model), with columns tool-use
  / vision / reasoning / context window and a leading ✓/⚠ agent-readiness marker
  (⚠ when `catalog.Model.AgentWarnings` is non-empty). A trailing
  `N providers, M agent-ready` summary. Like single `--caps`, it makes **no
  network call and needs no credentials** — capabilities are static catalog
  facts. `--json` emits the `[]jsonCaps` array. Unsupported families are skipped.
  Always exits 0 (it's a survey; the single-provider `--caps` keeps exit-3 for
  CI gating).
- The previous "`--caps` is single-provider only" guard is relaxed to allow
  `--all`; `--caps` still can't combine with `--bench`/`--stream` (those make a
  live call, which `--caps` deliberately doesn't).

No `go.mod` change, no daemon change — a CLI-side static catalog read.

## Proven

- **Unit (`cmd/agt`):** over a three-provider synthetic catalog (one tool-less,
  one tool+vision+reasoning, one *unsupported family*) — the human matrix
  includes both supported providers, **skips** the unsupported one, and reports
  `2 providers, 1 agent-ready`; the `--json` form returns exactly the two
  supported rows with correct `tool_call`/`vision`/`warnings` per provider.
- **Live (network-free, custom catalog):** a three-provider catalog renders
  `⚠ acme / ✓ good / ✓ olla` with per-column yes/- and context sizes, plus
  `3 providers, 2 agent-ready`; `--json` emits the array with each model's
  warnings.

2 new tests; suite **1199** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc — capability surfaces, complete

| M | Surface |
|---|---|
| 23 | inspect one model (`--caps`) |
| 24 | advise at boot (banner) |
| 25 | enforce (Governor strict gate) |
| 26 | diagnose (`agt doctor` / `agt status`) |
| **27** | **compare all (`--caps --all` matrix)** |

The catalog's capability data — long inert — is now inspectable, comparable,
advised, diagnosed, and (opt-in) enforced. There is no remaining touchpoint at
which an operator is left guessing whether a model can drive the agent loop.

## Deferred — named

- **All-models-per-provider** — the matrix shows one model per provider; a
  per-provider model expansion (`--caps <id> --all`) would list every model of a
  single provider. Not yet wired; the cross-provider view is the more common need.
- **Down-routing** (M25) — the routing change to auto-pick a tool-capable model
  remains the larger deferred item.
