# Phase Report — Milestone M37 (Capability down-routing)

> Status: **shipped** · Date: 2026-05-31
> SPEC-15 (provider/model capability). A **fresh axis turn**: the
> resilience/observability axis (M28–M36) had matured, so this milestone returns
> to the provider/model capability arc (M23–M27) and completes its missing verb —
> *route*, not just reject.

## Why

The M23–M27 arc gave the capability surface five verbs: inspect (M23), advise
(M24), enforce (M25), diagnose (M26), compare (M27). Enforcement (M25) is binary:
a tools-bearing request to a tool-incapable model is *rejected* pre-flight. But an
operator who configured a small, cheap default model that happens to lack tool-use
doesn't want the run to die — they want it to *work*, ideally on a capable model
from the same provider they already pay for. M37 adds that: **down-routing** remaps
the request to a tool-capable alternative instead of rejecting it.

## What shipped

- **`catalog.ToolCapableAlternative(modelID) (alt, ok)`
  (`kernel/catalog/types.go`)** — finds a tool-capable substitute within the
  **same provider** as `modelID`. Among that provider's tool-capable models
  (excluding the model itself) it picks the largest context window, tie-broken by
  model id ascending (deterministic despite random map iteration). Same-provider
  only, so the substitute is on a provider that is already configured and
  credentialed — a cross-provider remap could route to an unregistered provider.
- **Governor config + pre-flight remap (`kernel/governor/governor.go`)** — new
  `DownRouteToolModels bool` + `ToolCapableAlternative func(model)(string,bool)`.
  In `Complete`, **before** the M25 strict gate, when down-routing is on and a
  tools request targets a known tool-incapable model, the Governor looks up an
  alternative and, if found, rewrites `req.Model` and journals a
  `capability.rerouted` event (`{from_model, to_model, capability,
  tools_requested}`). A successful remap means the strict gate then sees a capable
  model and passes; a miss leaves `req.Model` unchanged so the gate (if strict)
  still rejects.
- **New event kind `event.KindCapabilityRerouted` (`capability.rerouted`)** —
  registered in the validation map; the audit counterpart to M25's
  `capability.rejected`.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_MODEL_DOWNROUTE=on`;
  `ToolCapableAlternative` backed by `cat.ToolCapableAlternative`; governor banner
  gains `tool-downrouting`. Per-tenant governors inherit it (the whole Config is
  copied by `WithLimits`).

## Design decisions

- **Same-provider only.** The substitute must be reachable. The provider currently
  serving the (incapable) model is registered and credentialed; one of its other
  models almost certainly is too. A cross-provider remap would have to verify the
  target provider is registered + credentialed — more surface, more failure modes.
  In-provider is the safe, high-value 80%.
- **Largest-context sibling.** When a provider offers several tool-capable models,
  "down-route" should land on the *most* capable one, not a random sibling — so the
  heuristic is largest context window, with an id tie-break for determinism.
- **Before the strict gate, composes with it.** Down-routing and strict mode are
  orthogonal flags that combine into the natural policy: *reroute if possible, else
  reject*. Down-routing alone (no strict) simply upgrades the model when it can and
  otherwise lets the existing advisory path handle it.
- **Journaled remap.** Silently swapping the model would make `agt why` lie about
  what ran. `capability.rerouted` records the swap so the served-vs-requested model
  difference is auditable — mirroring how `capability.rejected` records a block.
- **Off by default.** Like every capability enforcement knob (M25), down-routing
  changes routing behaviour, so it's opt-in.

## Tests

- `kernel/catalog/downroute_test.go` — `ToolCapableAlternative` picks the
  largest-context capable sibling; tie-breaks by id (looped to defeat map-order
  randomness); returns none when no capable sibling exists; handles an unknown
  model; never reroutes a model to itself.
- `kernel/governor/governor_test.go` — down-routing remaps `req.Model` (the
  provider sees the new model) and journals `capability.rerouted from→to`; with no
  alternative it falls through to the strict gate's reject (composition with M25);
  with the flag off the model is left unchanged.

Test count: **1240 → 1248**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (custom catalog: weak-1 [no tools] + strong-1 [tools])

```
A) strict only (AGEZT_MODEL_STRICT=on), model=weak-1, request carries tools:
   agt run "list files"
   → governor: model does not support tool-use: model "weak-1" …
   journal: 1 capability.rejected

B) down-routing on (… + AGEZT_MODEL_DOWNROUTE=on):
   banner: governor … tool-downrouting
   agt run "list files"
   → proceeds to the provider (then fails dialing the black-hole endpoint)
   journal: 1 capability.rerouted   {"from_model":"weak-1","to_model":"strong-1"}
```

Same incapable model + same tools request: strict-only rejects it pre-flight;
down-routing rewrites it to the capable sibling `strong-1` and lets the run
proceed — proving the remap end-to-end (the subsequent black-hole dial failure is
expected and unrelated).

## What's next

The capability arc now has six verbs (inspect/advise/enforce/diagnose/compare/
route). Remaining capability frontiers and fresh axes:

1. **Cross-provider down-routing** (MED) — extend M37 to a tool-capable model on a
   *different* registered+credentialed provider when the same provider has no
   capable sibling. Needs the alternative-finder to know which providers are
   registered (pass the governor's registered set, or filter in the daemon).
2. **Down-routing in `agt provider check`** (LOW) — surface, in the capability
   matrix (M27), which incapable models *would* down-route and to what, so the
   operator sees the safety net before a run hits it.
3. **Vision/attachment enforcement** (MED) — M25 only gates tool-use; extend the
   strict gate + down-routing to image/attachment modalities once the agent
   message type carries images (deferred from M23–M27).
