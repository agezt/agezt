# M380 — Tool-call policy card in the run-detail view (SPEC-12 §4 / SPEC-07)

## SPEC audit (read-vs-code)
SPEC-12 §4's tool-call card is "expandable: input/output/isolation/**policy**/
cost — the debug view (SPEC-07)". M379 added the **isolation** leg; this adds the
**policy** leg.

**Verified gap:** the agent loop journals a `policy.decision` event for *every*
tool call (agent.go:644) — payload `{tool, call_id, capability, allow, reason,
would_ask, hard_denied}` — but the dashboard's `arcDetail` had **no case** for
it, so in the run-detail arc it rendered as a bare kind line (the `default`
returned only the subject). The verdict (allow/deny, would-ask, hard-deny, the
reason) was in the journal but invisible in the run timeline. Offline-verifiable,
SPEC-12 §4 parity.

## What
- **`kernel/webui/dashboard.html`** — `arcDetail`/`arcFull` gained a
  `policy.decision` case:
  - compact: `✓ allow <cap> · would-ask — <reason>` / `✗ deny …` /
    `✗ HARD-DENY …` (the hard-deny floor is called out distinctly).
  - expandable: decision (allow/deny/hard-deny), capability, tool, would-ask
    note, reason.
  - XSS-safe: textContent via `el()` only; `TestDashboard_NoUnsafeDOMSinks`
    still green.

## Verification
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersPolicyCard` — asserts
  the embedded HTML carries the case + the payload keys it reads
  (`policy.decision`, `hard_denied`, `would_ask`, `capability`).
- **Negative control:** renaming `hard_denied` → `hard_DELETED` in the HTML →
  the test FAILs on the missing marker; restored byte-identical.
- **Live demo** (mock provider, `AGEZT_DEMO_LOOP`): a real run's
  `policy.decision` events render in the run-detail card —
  compact `✓ allow shell · would-ask — level L2; AskPolicy=AskAllow (would prompt
  in MVP)`; expanded `policy / decision: allow / capability: shell / tool: shell
  / would ask: yes (Ask folded to Allow) / reason: …` (Playwright).
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2150** passing (was 2149; +1), 0 failures. CHANGELOG (Added).

## Scope notes
- The run-detail tool-call card now covers input/output (M336/M341), **isolation**
  (M379), **policy** (M380), and cost (budget.consumed in the run header + arc) —
  the SPEC-12 §4 / SPEC-07 debug-view legs that are realizable on the vanilla-JS
  Live Monitor. The full widget SDK / sandboxed-iframe / marketplace remain
  Phase-5–8, out of offline scope (recorded in next.md).
