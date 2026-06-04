# M385 — Render Governor decision events in the run-detail view (SPEC-07 Live Monitor)

## Audit (read-vs-code)
M382 linked the Governor's per-call decision events to their run, so they now
appear in the run-detail journal slice. But the dashboard's `arcDetail` had no
case for any of them — `routing.decision`, `provider.fallback`,
`capability.degraded` (M381), `capability.rerouted`, `capability.rejected`,
`rate.limited`, `budget.exceeded` — so each rendered as a bare kind line, hiding
the routing/spend story that was now reachable. Verified by reading `arcDetail`
(cases existed only for tool/policy/warden/llm/task/budget.consumed).

## What
- **`kernel/webui/dashboard.html`** — added `arcDetail` cases for the seven
  Governor decision events, each a compact one-liner from the journal payload:
  - `routing.decision` → `routed → <primary> (chain: …) · <model>`
  - `provider.fallback` → `fallback <failed> → <next>: <reason>`
  - `capability.degraded` → `⚠ <capability> degraded on <model> — <reason>`
  - `capability.rerouted` → `rerouted <from> → <to> (<capability>)`
  - `capability.rejected` → `✗ rejected — <model> lacks <capability>`
  - `rate.limited` → `rate-limited (used N/limit per min)`
  - `budget.exceeded` → `budget exceeded (<scope>) $spent / $ceiling`
  XSS-safe: textContent via `el()` only (the `TestDashboard_NoUnsafeDOMSinks`
  guard still passes).

## Verification
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersGovernorEvents` — asserts
  each kind + the payload keys it reads (`primary`, `from_model`, `to_model`,
  `spent_microcents`, `limit_per_min`) are present in the embedded HTML.
- **Negative control:** renaming `from_model` → `from_DELETED` → the test FAILs on
  the missing marker; restored byte-identical.
- **Live demo** (mock provider, `AGEZT_DEMO_FAIL_PRIMARY=1` to force a fallback,
  Playwright): the run-detail card shows
  `routing.decision ⟶ routed → mock-failshim (chain: mock-failshim, mock) · mock`
  and `provider.fallback ⟶ fallback mock-failshim → mock: demo-shim: simulated
  primary failure`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2167** passing (was 2166; +1). CHANGELOG (Added, user-visible).

## Scope notes
- This closes the M381/M382-recorded follow-up (render the governor events in the
  run-detail card). The run-detail surface now covers: tool input/output
  (M336/M341), isolation (M379), policy (M380), context (M373), and the full
  Governor routing/capability/spend decision set (M385).
