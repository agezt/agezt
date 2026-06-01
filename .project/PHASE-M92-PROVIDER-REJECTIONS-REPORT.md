# Phase Report — Milestone M92 (`agt provider rejections`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-14 capability observability.

## Why

Four milestones now emit capability-gating events — the M25 strict tool gate, the
M40 down-route, and the M91 vision gate all journal `capability.rejected` /
`capability.rerouted` — but nothing surfaced them. An operator whose run was
blocked ("model does not support vision / tool_call") had no audit of how often
the capability gates fire or which models trip them. M92 adds `agt provider
rejections`, completing the capability-enforcement story: M23/M25/M40/M91
*enforce*; this *surfaces* what they did.

## What shipped

- **Server `handleProviderRejections`** — folds `capability.rejected` (model +
  capability: tool_call/vision) and `capability.rerouted` (from_model → to_model
  for the down-route) into one newest-first timeline, limited, with the shared
  `--since` window.
- **CLI `agt provider rejections [N] [--since <dur>] [--json]`** — renders
  `REJECTED <capability> on <model>` and `rerouted <capability> <from> → <to>`.

## Design decisions

- **One surface for both gating outcomes.** A rejection (hard block) and a
  reroute (silent remap to a capable model) are the two ways a capability gate
  resolves; showing both in one view answers "what did the gates do?" completely.
- **Provider-namespaced.** Capability gating is about model fitness, so it sits
  under `agt provider` alongside `check`/`log`/`stats`.

## Tests

- `TestProviderRejections_FoldsCapabilityEvents` — a tool_call rejection + a
  vision rejection + a reroute: all three newest-first (reroute first), and the
  vision rejection on `mock` is present.

Test count: **1336 → 1337**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run --image a.png "describe"     # x2 — M91 gate rejects each (mock = no vision)
$ agt provider rejections
  2026-06-01 17:24:36  REJECTED  vision     on mock
  2026-06-01 17:24:36  REJECTED  vision     on mock
```
