# Phase Report — Milestone M25 (Strict model-capability enforcement)

> Status: **shipped** · Date: 2026-05-31
> SPEC-15 §1. The enforcement step after the M23/M24 advisories: opt-in, the
> Governor now *rejects* a tools-bearing request to a model the catalog knows
> can't call tools, before any provider is touched.

## Why

M23 made model capabilities inspectable (`agt provider check --caps`); M24 put
the headline tool-use gap in the boot banner. Both *inform*. M25 adds the option
to *enforce*: when an operator wants fail-fast guarantees, a request that carries
tools but targets a tool-incapable model should error clearly up front rather
than fail deep inside the provider call with a cryptic upstream message.

The reason it's **opt-in, not default** is catalog-data quality. A model flagged
`tool_call=false` in the catalog might actually support tools (locally-served
models are routinely under-flagged). Hard-blocking on possibly-stale metadata
would break working setups. So M25 ships three safety rails:

1. **Off by default** (`AGEZT_MODEL_STRICT=on` to enable) — the advisories (M24)
   remain the default, non-blocking behaviour.
2. **Only blocks *known* models** — if the catalog doesn't know the model, the
   request is never blocked. A catalog gap can't wedge a working deployment.
3. **Only tools-bearing requests** — a plain completion to a non-tool model is
   fine and still passes.

## What shipped

- **Governor (`kernel/governor`)** — two `Config` fields:
  `StrictModelCapabilities bool` and `ModelToolCapable func(model) (capable,
  known bool)` (injected, so the Governor stays decoupled from
  `kernel/catalog`). In `Complete`, immediately after the task-model override
  resolves the final `req.Model`, a tools-bearing request to a *known,
  tool-incapable* model is rejected with the new `ErrModelLacksToolUse` sentinel
  and a `capability.rejected` event — before rate, budget, routing, or any
  provider call. Per-tenant governors inherit the config via `WithLimits` (it
  copies the whole `Config`).
- **Event (`kernel/event`)** — `KindCapabilityRejected` (`capability.rejected`),
  registered.
- **Daemon (`cmd/agezt/main.go`)** — `AGEZT_MODEL_STRICT=on` sets the flag and
  wires `ModelToolCapable` from the catalog (`cat.FindModel`); the governor
  banner appends `, strict-capabilities` when on.

No `go.mod` change. The gate is a pure pre-flight branch; when off it is a single
boolean check with zero behavioural change.

## Proven

- **Unit (governor):** strict + tools + known-non-tool model → `ErrModelLacksToolUse`,
  the provider is **not** called, and a `capability.rejected` event is journaled;
  no tools → passes; unknown model → not blocked; strict off → passes; tool-capable
  model → passes.
- **Live (daemon, custom catalog):** booting `AGEZT_MODEL_STRICT=on
  AGEZT_MODEL=acme-mini` (a `tool_call=false` model) shows
  `governor: …, strict-capabilities` plus the M24 advisory; `agt run "say hello"`
  is rejected with `governor: model does not support tool-use: model "acme-mini"
  (request carries 7 tool(s))` — a pre-flight error, **not** a connection error
  to the (fake) provider URL — and the journal holds the `capability.rejected`
  event. Rebooting **without** strict: the banner drops `strict-capabilities` and
  the same run flows through the chain (falling back to the offline mock) — no
  rejection. The opt-in contract holds both ways.

5 new tests; suite **1196** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc — the capability triad

| M | Layer | Behaviour |
|---|---|---|
| 23 | inspect | `agt provider check --caps` — on-demand capability view + warnings |
| 24 | advise | boot banner — automatic advisory for the selected model |
| **25** | **enforce** | **opt-in strict gate — reject tools→non-tool-model pre-flight** |

The catalog's capability data is now an inspectable fact, a boot-time signal, and
(when the operator opts in) a hard guarantee — closing the loop from "the catalog
knows" to "the system won't let you mis-route."

## Deferred — named

- **Down-routing** — instead of rejecting, route a tools request to a
  tool-capable fallback. Needs per-provider model remapping (today every chain
  member sees the same `req.Model`), so it's a larger change; rejection is the
  correct, simple semantics for now.
- **Other capabilities** — vision/attachment enforcement once the agent message
  type carries image content end-to-end (today the loop is text).
- **Per-task-route strictness** — `AGEZT_TASK_MODEL_OVERRIDES` can swap the model
  per task type; the gate already checks the *final* `req.Model`, so this works,
  but a per-route strictness toggle could be finer-grained.
