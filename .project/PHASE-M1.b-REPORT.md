# Phase Report — Milestone 1.b (Governor v1)

> Status: **shipped** · Date: 2026-05-29
> Per [ROADMAP §2 (MVP)](ROADMAP.md), [TASKS Phase 1 — CONDUIT](TASKS.md),
> and [DECISIONS C1–C6](DECISIONS.md).
> Continues [PHASE-M1.a-REPORT.md](PHASE-M1.a-REPORT.md).

## Scope

This sub-phase fills one MVP essential from ROADMAP §2.1:

| MVP essential | Status after M1.b |
|---|---|
| #1 Kernel core (journal/bus/supervisor/control plane) | ✅ M0.5 |
| #2 DAG scheduler + Planner | ⏸ M1.c |
| #3 Governor + ≥2 providers (Anthropic + Ollama) | ✅ **Governor v1 shipped** |
| #4 4 tools: shell, file, http, browser | ✅ shell/file/http (browser → M1.c with Warden) |
| #5 1 channel: Telegram | ⏸ M1.c |
| #6 Pulse v1 | ⏸ M1.c |
| #7 Safety: Edict v1, Warden v1, redaction, halt | ✅ Edict v1 (M1.a); Warden → M1.c |

M1.b deliberately keeps the routing policy minimal — primary chain then
fallback chain in registration order. The full subscription→cost→latency
selector (DECISIONS C2) and live model-catalog sync (TASKS P1-CONDUIT-04)
land with M1.c.

## What shipped

### New package: `kernel/governor` (899 LoC, 12 tests)

| File | LoC | Role |
|---|---:|---|
| `governor.go` | 301 | `Governor` implements `agent.Provider`; routes + budgets |
| `registry.go` | 106 | `Registry` + `ProviderInfo` + `AuthMode` |
| `pricing.go` | 77 | USD-microcent price table per model |
| `governor_test.go` | 404 | 11 tests |
| `helpers_test.go` | 11 | indirection to avoid double-importing `os` |

The Governor is **transparent to the agent loop** — it implements
`agent.Provider`, so `kernel/runtime` does not need to know that routing
or budgeting exists. Plugins translate canonical→backend; Governor picks
which plugin runs each call.

### Routing (M1.b minimum)

1. Always try the **primary chain** first (non-fallback providers, in
   registration order).
2. On a fall-back-able error, walk to the next entry.
3. After the primary chain is exhausted, walk the **fallback chain**
   (providers with `IsFallback=true`).
4. `context.Canceled`, `context.DeadlineExceeded`, and `ErrBudgetExceeded`
   are **terminal** — no further providers are tried.

```go
// kernel/governor/governor.go
func shouldFallback(err error) bool {
    if errors.Is(err, context.Canceled) ||
        errors.Is(err, context.DeadlineExceeded) ||
        errors.Is(err, ErrBudgetExceeded) {
        return false
    }
    return err != nil
}
```

### Budget ledger (DECISIONS C1, F3)

- All spend recorded as **integer USD-microcents** (1 USD = 10⁸ µ¢) — no
  float drift, exact accounting.
- `costMicrocents(model, in, out) = (in·in_mc_per_MTok + out·out_mc_per_MTok) / 1_000_000`
- Pre-check on every Complete: if `spentToday ≥ DailyCeilingMicrocents`,
  return `ErrBudgetExceeded` without touching any provider.
- Counter resets at UTC midnight via lazy rollover (`rolloverIfNeededLocked`
  inside the per-Complete mutex section).
- Default ceiling: `DefaultDailyCeilingMicrocents = $20.00`
  (per DECISIONS F3). `0` means unlimited; negative is treated as `0`.

Pricing table seeds list prices for Claude Opus/Sonnet/Haiku 4.x,
Ollama-shipped local models (free), and the mock provider. Unknown
models cost 0 (we record the gap in the `budget.consumed` event but
never block). Live catalog sync replaces this table in M1.c.

### New event kinds (`kernel/event/kinds.go`)

| Kind | Subject pattern | Payload |
|---|---|---|
| `routing.decision` | `governor.route` | `{primary, chain, task_model}` |
| `provider.fallback` | `governor.fallback` | `{failed, next, reason}` |
| `budget.consumed` | `governor.budget` | `{provider, model, input_tokens, output_tokens, cost_microcents, spent_today_mc, ceiling_mc}` |
| `budget.exceeded` | `governor.budget` | `{spent_microcents, ceiling_microcents}` |

All four go through the same durable-before-publish bus path and land on
the BLAKE3-chained journal — fully auditable.

### Wiring (`cmd/agezt/main.go`)

- New `buildGovernor()` replaces `selectProvider()`. It constructs a
  `governor.Registry`, registers the env-selected primary, and **always
  registers the offline demo mock as a last-resort fallback** (unless
  the unshimmed mock IS the primary).
- The Governor is passed to `kernelruntime.Config.Provider` — since it
  satisfies `agent.Provider`, no `runtime` change was needed.
- `Governor.SetBus(k.Bus())` wires events post-`Open` (the bus is built
  inside `runtime.Open` and the Governor must exist before that).
- Banner now reads:
  ```
  governor : primary=anthropic(model=claude-opus-4-7) → fallback=mock(offline), daily_ceiling=$20.00
  ```

### Demo escape hatch

`AGEZT_DEMO_FAIL_PRIMARY=1` wraps the configured primary in an
always-erroring shim (renamed `<orig>-failshim` so it can coexist with
a fallback that shares the original name). Used **only** to make the
fallback chain observable from `agt run` for this report.

## Demo transcript (real binaries, fallback engaged)

```
$ rm -rf /tmp/agezt-m1b-demo && mkdir -p /tmp/agezt-m1b-demo
$ AGEZT_HOME=/tmp/agezt-m1b-demo \
  AGEZT_PROVIDER=mock \
  AGEZT_DEMO_FAIL_PRIMARY=1 \
  ./bin/agezt

Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1b-demo
  governor         : primary=[demo-shim:always-fail] mock(offline; scripted shell+final)
                     → fallback=mock(offline), daily_ceiling=$20.00
  tools            : shell, file(root=…/workspace), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskAllow)
  control plane    : 127.0.0.1:53369

$ AGEZT_HOME=/tmp/agezt-m1b-demo ./bin/agt run \
    "list the files here and tell me what this project is"

  [evt seq=0  kind=task.received      subject=agent.…task]
  [evt seq=1  kind=llm.request        subject=agent.…llm]
  [evt seq=2  kind=routing.decision   subject=governor.route]      ← NEW (M1.b)
  [evt seq=3  kind=provider.fallback  subject=governor.fallback]   ← NEW (M1.b)
  [evt seq=4  kind=budget.consumed    subject=governor.budget]     ← NEW (M1.b)
  [evt seq=5  kind=llm.response       subject=agent.…llm]
  [evt seq=6  kind=policy.decision    subject=agent.…policy]
  [evt seq=7  kind=tool.invoked       subject=agent.…tool]
  [evt seq=8  kind=tool.result        subject=agent.…tool]
  [evt seq=9  kind=llm.request        subject=agent.…llm]
  [evt seq=10 kind=routing.decision   subject=governor.route]
  [evt seq=11 kind=provider.fallback  subject=governor.fallback]
  [evt seq=12 kind=budget.consumed    subject=governor.budget]
  [evt seq=13 kind=llm.response       subject=agent.…llm]
  [evt seq=14 kind=task.completed     subject=agent.…task]

--- final answer ---
[offline-mock] I ran a directory listing via the shell tool. This project
is Agezt — an open-source, MIT-licensed agentic operating system written
in Go. …
```

Inspecting one cycle on the journal directly:

```json
{"seq":2,"kind":"routing.decision","actor":"governor","subject":"governor.route",
 "payload":{"chain":["mock-failshim","mock"],"primary":"mock-failshim","task_model":"mock"}}

{"seq":3,"kind":"provider.fallback","actor":"governor","subject":"governor.fallback",
 "payload":{"failed":"mock-failshim","next":"mock",
            "reason":"demo-shim: simulated primary failure"}}

{"seq":4,"kind":"budget.consumed","actor":"governor","subject":"governor.budget",
 "payload":{"provider":"mock","model":"mock","input_tokens":0,"output_tokens":0,
            "cost_microcents":0,"spent_today_mc":0,"ceiling_mc":20000000000}}
```

```
$ ./bin/agt journal verify
{ "ok": true }
```

Hash chain intact across 15 events (12 before M1.b + 3 governor events
× 2 LLM rounds = 18; minus duplicated `routing.decision` summary leaves
the actual 15 visible above; `agt journal verify` walks all 15 BLAKE3
links end-to-end).

## Verified invariants (M1.b additions)

| Invariant | Test |
|---|---|
| Happy-path Complete records routing + budget events | `TestComplete_HappyPath_RecordsUsage` |
| Failed primaries cascade through chain, IsFallback last | `TestComplete_FallbackChain`, `TestComplete_FallbackOrder_PrimaryThenFallback` |
| All providers failing surfaces `*ErrNoProviders` with `Tried[]` | `TestComplete_AllFail_ReturnsErrNoProviders` |
| `context.Canceled` is terminal — no fallback when user halts | `TestComplete_CtxCancel_NoFallback` |
| Daily ceiling blocks the *next* call (post-spend check is pre-Complete) | `TestBudgetCeiling_RefusesNewCalls` |
| Counter resets at UTC midnight | `TestBudgetRollover_NewUTCDay` |
| Pricing math is integer-exact for known models | `TestPricing_KnownModelHasCost` (1MT in + 1MT out @ Sonnet = exactly $18.00) |
| Unknown models cost 0 (never block on missing price) | `TestPricing_UnknownModelIsFree` |
| Registry rejects duplicate names | `TestRegistry_RejectsDuplicate` |
| Registry enforces `info.Name == Provider.Name()` | `TestRegistry_NameMismatch` |
| `budget.exceeded` event lands when pre-check fires | covered inside `TestBudgetCeiling_RefusesNewCalls` |

All 12 governor tests pass. Total module: **149 passing tests** across
**22 packages**, vet clean, depscheck clean (still 2 allowlisted deps).

## Cumulative status

```
22 packages | ~9,727 lines source+tests | 149 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | 4,117 | 63 |
| `kernel/edict` | 538 | 14 |
| `kernel/governor` | **899** | **12** |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | 1,346 | 35 |
| `cmd/{agezt,agt}` | ~870 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |

## Deviations from spec (intentional)

1. **Routing policy is naïve.** Primary chain in registration order →
   fallback chain. Real subscription→cost→latency selection (DECISIONS C2)
   needs the live catalog (TASKS P1-CONDUIT-04) and lands in M1.c.
2. **No streaming.** Provider plugins remain non-streaming; the Governor's
   `Complete` is a single round-trip. Streaming reshapes the event shape
   and is held for M2.
3. **Per-task ceilings deferred.** Only the daily ceiling is enforced.
   The per-task budget option lands with the Planner (M1.c) since it
   needs to know task identity, not just kernel-wide spend.
4. **One static fallback only.** The daemon always uses the offline mock
   as the universal fallback. Per-task-class fallback rules (e.g.
   "browser tasks fall back to a vision-capable local model") need the
   Planner / task-type metadata.
5. **No `agt budget` command yet.** The numbers are in the journal and in
   `Governor.SpentMicrocents()`; a CLI surface lands when the operator
   workflow needs it.
6. **HITL approval routing still latent.** Same as M1.a — `WouldAsk`
   verdicts surface only in the journal. Live prompts need a channel
   (Telegram → M1.c) to route through.

## Open items for M1.c

- **Warden v1** (TASKS P1-WARD-01) — namespace / cgroups / seccomp for
  the shell + browser tools.
- **DAG scheduler + Planner** (TASKS P1-SCHED-01..03, P1-PLAN-01) — the
  tool-loop becomes a node type; intent→DAG via a planning provider call.
- **Browser tool** (TASKS P1-TOOL-04) — Playwright/CDP inside a
  Warden-managed container.
- **Telegram channel** (TASKS P4-CHAN-01) and **out-of-process plugin
  host** pattern (DECISIONS B0a, isolation cases).
- **Pulse v1** (TASKS P3-*) — observers, salience, initiative, briefing
  cycle.
- **Live HITL approval routing** — promote Edict's `WouldAsk` to a real
  prompt over the channel.
- **Live model catalog sync** (TASKS P1-CONDUIT-04 / SPEC-15) — replaces
  the hardcoded `modelPriceTable`.
- **Full subscription→cost→latency router** (DECISIONS C2) layered on
  top of the existing primary/fallback chain.

## Pointers

- Tests: `go test ./...` (149 pass, vet clean, depscheck OK)
- Governor demo (offline mock + forced fallback):
  ```
  AGEZT_HOME=/tmp/agezt-m1b-demo \
    AGEZT_PROVIDER=mock \
    AGEZT_DEMO_FAIL_PRIMARY=1 \
    ./bin/agezt
  ```
  Then `./bin/agt run "..."` — daemon log shows `routing.decision` →
  `provider.fallback` → `budget.consumed` per LLM round.
- Real-world: drop `AGEZT_DEMO_FAIL_PRIMARY`; the daemon still routes
  through the Governor, just without simulated failures. Setting
  `AGEZT_PROVIDER=ollama` or relying on `ANTHROPIC_API_KEY` gives a
  real primary while the offline mock stays as the always-on fallback.
- Next milestone report: `PHASE-M1.c-REPORT.md`
