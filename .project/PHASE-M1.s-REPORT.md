# Phase Report — Milestone 1.s (subscription-first routing)

> Status: **shipped** · Date: 2026-05-29
> Per [DECISIONS C2](DECISIONS.md) and the M1.b `routeChain` TODO.
> Continues [PHASE-M1.r-REPORT.md](PHASE-M1.r-REPORT.md).

## Scope

The Governor has shipped since M1.b with a placeholder routing
policy: "primary list in insertion order, then fallback list."
DECISIONS C2 always called for **subscription → local → API-key**
preference within the primary chain, deferred until the catalog
sync and AuthMode tagging landed. Both are in (M1.n catalog sync,
M1.o vault). M1.s closes the loop.

Concretely: when a user has an Anthropic Pro subscription *and* an
ANTHROPIC_API_KEY both surfaced as primary providers, the
Governor must call the subscription provider first — the user
already paid for it, the marginal cost is $0. Only on a
fall-back-able failure should the chain walk down to the
pay-per-token API-key provider.

| Concern | Status |
|---|---|
| `routeChain` sorts primary providers by AuthMode priority | ✅ |
| Subscription routed before API-key when both present | ✅ tested |
| Local routed between subscription and API-key | ✅ tested |
| Within-tier order preserved (stable sort) | ✅ tested |
| Fallback chain order unchanged (insertion order, last) | ✅ unchanged |
| Sorting happens per-call (dynamic across hot reloads) | ✅ by design |
| `ErrNoProviders.Tried` reflects the chosen chain order | ✅ tested |
| Unknown AuthMode folds to API-key tier (cost-conservative) | ✅ |

## Changes

### 1. `kernel/governor/governor.go` — sort in `routeChain`

```go
func (g *Governor) routeChain(req agent.CompletionRequest) []*ProviderInfo {
    primary := make([]*ProviderInfo, len(g.primary))
    copy(primary, g.primary)
    slices.SortStableFunc(primary, func(a, b *ProviderInfo) int {
        return authModePriority(a.AuthMode) - authModePriority(b.AuthMode)
    })
    chain := make([]*ProviderInfo, 0, len(primary)+len(g.fallback))
    chain = append(chain, primary...)
    chain = append(chain, g.fallback...)
    return chain
}

func authModePriority(m AuthMode) int {
    switch m {
    case AuthSubscription:
        return 0
    case AuthLocal:
        return 1
    case AuthAPIKey:
        return 2
    default:
        return 2  // unknown → cost-conservative
    }
}
```

Three design choices worth recording:

**Why `slices.SortStableFunc`, not a one-time sort at `New()`.**
Auth mode is a property of `ProviderInfo`, and `Governor.Replace`
(from M1.r) can swap the entry for a name. If we sorted once at
construction, a creds rotation that promotes a provider from
API-key to subscription wouldn't change routing until the daemon
restarted — defeating the M1.r premise. Sorting per call is O(n
log n) on a list of ≤ 10 providers (sub-microsecond) and keeps
behaviour dynamic.

**Why stable sort.** Two providers in the same tier (two API-key
providers, e.g. an OpenAI key and a Mistral key) must be tried in
the order the operator registered them — that's the only signal we
have for their preference within a tier. `slices.Sort` doesn't
guarantee stability; `slices.SortStableFunc` does.

**Why unknown AuthMode → API-key tier.** A typo'd AuthMode value
in a future plugin would otherwise route to a random tier. Folding
to the most-expensive tier means a misconfigured provider is
*tried last*, not *tried first* — same fail-safe direction as the
budget caps (DECISIONS F3).

### 2. `kernel/governor/governor_test.go` — 3 new tests

[governor_test.go:562-585](../kernel/governor/governor_test.go#L562)
`TestGovernor_RoutesSubscriptionBeforeAPIKey` — registers an
API-key provider first, then a subscription provider. Without
sorting, insertion order would call the API-key one. Assertion:
`subProv.calls == 1`, `apiProv.calls == 0`.

[governor_test.go:587-627](../kernel/governor/governor_test.go#L587)
`TestGovernor_RoutesLocalAheadOfAPIKeyButBehindSubscription` —
all three tiers, all set to fail, so Complete walks the entire
chain. Verifies both that each provider is tried once and that
`ErrNoProviders.Tried` lists them in `[sub, loc, api]` order.
The `Tried` assertion matters because it's the operator-facing
signal — a failure log saying "tried sub, then loc, then api"
must match what actually happened.

[governor_test.go:629-654](../kernel/governor/governor_test.go#L629)
`TestGovernor_StableSortWithinSameTier` — two API-key providers
registered a-then-b, with `a` failing. The chain must try `a`
first (registration order), then fall back to `b`. Catches a
regression where someone swaps `SortStableFunc` for `SortFunc`
and the tier-internal order goes non-deterministic.

## Test summary

```
go test ./... -count=1
ok  	github.com/agezt/agezt/kernel/governor	0.099s
```

Total suite: **431 passing** (from 427 after M1.r). +3 from the
new subscription-routing tests; +1 from earlier work that landed
in the same window.

## Behaviour by example

Operator has both an Anthropic OAuth subscription and an
`ANTHROPIC_API_KEY` set. Catalog sync (M1.n) registers them as two
separate `ProviderInfo` entries, AuthMode=Subscription and
AuthMode=APIKey respectively. After M1.s:

```
agt run --task chat 'hello'
→ Governor.routeChain returns [anthropic-sub, anthropic-key]
→ Complete tries anthropic-sub first   ← $0 marginal cost
→ if 429 / 5xx / refresh-failed → falls back to anthropic-key
```

Before M1.s, the chain was `[anthropic-key, anthropic-sub]` (or
the reverse, depending on which catalog file loaded first) — a
random outcome where the operator could end up billed on the
metered key even though their flat-rate subscription was sitting
right there.

The CLI's `agt provider check --all` output (M1.p.x) already
groups by AuthMode visually, so operators can verify the routing
intent matches their setup.

## What's intentionally NOT in M1.s

DECISIONS C2 also calls for **cost-then-latency** within a tier,
which would replace the stable-sort tie-break. That requires:

1. Catalog-level cost annotations per model (currently optional
   and not exposed by all vendors' pricing pages).
2. A latency observation surface (probably the bus events from
   `provider check --all`, persisted somewhere).
3. A decision about what "cost" means for a subscription provider
   (probably "0 if within plan limits, else opportunity cost",
   which is a rabbit hole).

None of those exist. Insertion order is a serviceable proxy
("operator-curated preference") and the test suite locks it in,
so the operator's mental model from M1.b still works.

## Deferrals

- **Per-call routing overrides.** A future `RouteOptions.PreferredAuthMode` could let a task force "I want the cheap one" or "I want the highest-quality even if metered." Out of scope; the current `RouteOptions.PreferredProvider` already exists for the by-name case.
- **Cost-then-latency sort within tier.** As described above.
- **Per-task-type routing.** DECISIONS C2 implies different task types (chat vs. code vs. embedding) might prefer different tiers. Not in M1; will require a `TaskType` field on `RouteOptions`.
- **AWS Bedrock streaming.** Last remaining streaming adapter. Picked up next; see M1.t.

## Files touched

- [kernel/governor/governor.go](../kernel/governor/governor.go) — added `slices` import, replaced `routeChain` body, added `authModePriority`.
- [kernel/governor/governor_test.go](../kernel/governor/governor_test.go) — appended 3 tests.

No control-plane changes, no CLI changes, no catalog changes —
the wedge is entirely inside the Governor, which is the right
place for a routing policy decision.
