# M379 — Tool-call isolation card + correlated warden events (SPEC-12 §4 / SPEC-07)

## SPEC audit (read-vs-code)
SPEC-12 §4 lists, among first-party widgets, a **tool-call card** that is
"expandable: input/output/**isolation/policy/cost** — the debug view (SPEC-07)".
The web Live Monitor's run-detail arc already renders tool input/output
(M336/M341) and cost (budget.consumed), and policy denials surface in the
tool.result text. The missing leg was **isolation** — which Warden profile a
tool actually ran under.

**Verified gap (two layers, found via Playwright on a live run):**
1. The dashboard's `arcDetail` had **no case** for `warden.executed`, so the
   event rendered as a bare kind line with no detail — even though its payload
   carries `profile_effective` / `profile_requested` / `downgraded` / `argv0` /
   `exit_code`.
2. Deeper: the `warden.executed` (and `warden.profile_downgraded`) events were
   emitted with an **empty correlation id**. The shell tool built its
   `warden.Spec` with `Actor: "tool.shell"` but never set `CorrelationID`, so the
   isolation events were **orphaned from their run** — they never reached the
   run-detail view (filtered by correlation) and `agt why <event-id>` on one
   returned nothing. The `Spec.CorrelationID` field's own doc says it exists "so
   `agt why <id>` walks the chain back to the originating task" — intent unmet.

Both are offline-verifiable, priority-A (security observability: you couldn't
audit which isolation a specific run's tool calls used) + SPEC-12 §4 parity.

## What
- **`kernel/warden/warden.go`** — added `WithCorrelation(ctx, corr)` /
  `CorrelationFrom(ctx)` (mirrors the existing memory/worldmodel/skill ctx
  helpers exactly; the Warden engine already propagates `Spec.CorrelationID` →
  event, tested by `TestEvent_ExecutedPayloadShape`).
- **`kernel/runtime/runtime.go`** — `RunWith` now also sets
  `warden.WithCorrelation(runCtx, corr)` alongside the other three, so every
  run's tool ctx carries the correlation for warden-backed tools.
- **`plugins/tools/shell/shell.go`** — the Spec now sets `CorrelationID:
  warden.CorrelationFrom(ctx)`; empty (harmless) when run outside a kernel run.
- **`kernel/webui/dashboard.html`** — `arcDetail`/`arcFull` gained a
  `warden.executed` case: a compact `isolation <effective> ⚠ downgraded from
  <requested> · <argv0>` line and an expandable requested/effective/command/
  exit/duration block. XSS-safe (textContent via `el()` only; the
  `TestDashboard_NoUnsafeDOMSinks` guard still passes).

## Verification
- **Tests (+3):** `plugins/tools/shell` `TestShell_StampsRunCorrelationOnSpec`
  (a capturing fake Warden asserts the Spec gets the ctx correlation, and empty
  with no ctx — no panic); `kernel/warden` `TestWithCorrelation_RoundTrips`;
  `kernel/webui` `TestDashboard_RendersIsolationCard` (locks the render wiring +
  the warden.executed payload keys it reads).
- **Negative control:** neutering `profile_effective` in dashboard.html →
  `TestDashboard_RendersIsolationCard` FAILs on the missing marker; restored
  byte-identical.
- **Live end-to-end demo** (mock provider, `AGEZT_DEMO_LOOP` shell calls):
  - Journal: `warden.executed` (×5) + `warden.profile_downgraded` now carry the
    **same** `run-01KT9Y4G…` correlation as the run's `tool.invoked` (×5) — was
    empty before the fix.
  - `agt why <warden.executed id>` → resolves the full 118-event run correlation
    (was: nothing).
  - Playwright on the run-detail card: row renders `isolation none ⚠ downgraded
    from namespace · cmd`; expanded block shows `requested: namespace /
    effective: none ⚠ DOWNGRADED / command: cmd / exit code: 1 / duration: 18ms`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2149** passing (was 2146; +3), 0 failures. CHANGELOG (Added + Fixed,
  user-visible).

## Scope notes
- SPEC-12 (Widgets) is fundamentally a Phase-5–8 TS/React SDK + sandboxed-iframe
  widget system + marketplace — OUT of offline scope (no React/SDK here). Its
  realizable offline surface is the vanilla-JS Live Monitor's system widgets
  (tool-call card, config inspector M336, context inspector M373, and now the
  isolation card). The "policy" leg of the §4 tool-call card is partially present
  (denials surface in tool.result + the Policy panel/policy_log); a dedicated
  per-tool-call policy badge is a candidate follow-up. "cost" is per-LLM-call
  (budget.consumed), shown in the run header + arc.
- Audited SPECs now: 01–16 except 11 has no further offline milestone (audited:
  health/readiness/loopback done+tested; Docker/GHCR/k8s/OTel out-of-scope).
