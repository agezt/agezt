# Phase Report — Milestone M40 (Cross-provider down-routing)

> Status: **shipped** · Date: 2026-06-01
> SPEC-15 (provider/model capability). Completes the down-routing story M37 began:
> when a model's own provider can't supply a tool-capable substitute, cross to one
> that can.

## Why

M37 down-routing remaps a tools request from a tool-incapable model to a
tool-capable **sibling in the same provider** — deliberately conservative, because
the same provider is guaranteed registered and credentialed. But a common setup
has the default model on a provider with *no* tool-capable model at all (a
single-model local/cheap provider). There M37 finds no substitute and falls
through to the reject. M40 widens the search to a tool-capable model on a
**different** registered provider, so the run can still proceed.

## What shipped

- **`catalog.ToolCapableAlternativeAmong(modelID, providerEligible func(provID)
  bool) (alt, ok)` (`kernel/catalog/types.go`)** — the generalised finder. Pass 1
  prefers the model's own provider (stay in-provider); only if it has no eligible
  capable sibling does Pass 2 widen to every other eligible provider. Within the
  chosen scope the largest-context capable model wins, tie-broken by id ascending.
  M37's `ToolCapableAlternative` is refactored to delegate with `providerEligible =
  (provID == own provider)`, so its same-provider behaviour is byte-for-byte
  preserved (existing M37 tests pass unchanged). Shared selection extracted into
  `pickBestToolCapable`.
- **Accurate eligible set (`cmd/agezt/buildGovernor`)** — the daemon tracks which
  catalog providers it *actually registered* (`registered` map, populated as each
  primary/alternate registers), keyed by catalog provider id. The cross finder
  consults exactly this set, so a remap target is always a model some registered
  provider serves — `applyModelRoute`/`ProviderInfo.Serves` can route to it. A
  catalog provider that was filtered out (no creds / unsupported family) or failed
  to build is *not* eligible, so the remap can never point at an unreachable model.
- **`AGEZT_MODEL_DOWNROUTE_CROSS=on`** — enables cross-routing (implies
  down-routing). Banner shows `tool-downrouting(cross)`.

## Design decisions

- **Same-provider still preferred.** Crossing providers can change cost/latency
  characteristics, so the remap only leaves the provider when it must. Pass 1
  (same provider) always wins over Pass 2 (cross) — pinned by a test where a far
  larger cross-provider model loses to a modest in-provider sibling.
- **Eligibility = actually registered, not "in the catalog".** The subtle failure
  mode is remapping to a model on a provider the governor can't route to: then
  `applyModelRoute` is a no-op and the request runs on the *original incapable*
  provider with a model it doesn't serve → an upstream error. Tracking the real
  registration set (rather than re-deriving "supported + credentialed", which can
  diverge when `buildFromCatalog` fails) makes the remap target always reachable.
- **Largest-context, deterministic.** Same heuristic as M37, applied globally
  across eligible providers, with an id tie-break so the choice is stable.
- **Opt-in, layered.** `DOWNROUTE` = same-provider (M37); `DOWNROUTE_CROSS` = widen
  to other providers (M40, implies the former). Off by default.

## Tests

- `kernel/catalog/downroute_test.go` (M40 additions): same-provider sibling beats a
  bigger cross option; crosses when there's no in-provider sibling; never routes to
  an **ineligible** provider (even if it's the only capable one); picks the
  largest-context model among multiple eligible cross providers. The M37
  same-provider tests pass unchanged through the refactor.

Test count: **1255 → 1259**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (two providers: weakco [incapable only] + strongco [capable])

```
A) same-provider only (AGEZT_MODEL_DOWNROUTE=on), model=weakco/weak-1 + tools:
   agt run "list files"
   → governor: model does not support tool-use: model "weak-1"
   journal: 1 capability.rejected           ← weakco has no capable sibling

B) cross-provider (AGEZT_MODEL_DOWNROUTE_CROSS=on):
   banner: governor … tool-downrouting(cross)
   agt run "list files"
   → proceeds to the provider (then fails dialing the black-hole)
   journal: 1 capability.rerouted   {"from_model":"weak-1","to_model":"strong-1"}
```

Same request, same incapable single-model provider: same-provider down-routing
can only reject it; cross-provider routing rewrites it to `strong-1` on the
*other* registered provider and lets the run proceed — the remap crossing a
provider boundary, end-to-end.

## What's next

The capability arc (inspect/advise/enforce/diagnose/compare/route, now same- AND
cross-provider) is essentially complete for tool-use. Remaining frontiers:

1. **Vision/attachment capability enforcement + down-routing** (MED) — extend the
   M25 gate and M37/M40 routing beyond tool-use to image/attachment modalities,
   once the agent message type carries images.
2. **`agt --token <tok>` flag + `agt whoami`** (LOW) — ergonomics for the M38/M39
   auth model.
3. **Fresh axis — multi-agent orchestration** — the least-explored layer
   (`delegate` tool / sub-agents, `kernel/runtime/subagent.go`,
   `subagent.spawned`), much shallower than policy/capability/runs.
