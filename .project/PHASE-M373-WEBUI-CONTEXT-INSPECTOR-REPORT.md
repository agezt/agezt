# M373 — Web UI context inspector (SPEC-07 / SPEC-10 §3.5)

## SPEC audit (read-vs-code)
SPEC-10 §3.5 says context observability is "surfaced in the UI's **context
inspector** (SPEC-07)", and the active goal explicitly lists, under priority B,
"web UI konuşma+context-inspector yüzeyi Playwright ile" (the web-UI
context-inspector surface, Playwright-verified).

**Gap:** M372 added `context_chars` + `context_by_role` to the `llm.request`
event, but nothing rendered it — the web UI run-detail arc showed only the event
kind/subject for `llm.request`, so the operator couldn't see how big a call's
context was or where it came from. This is the matching SPEC-07 surface for the
M372 foundation.

## What
- **`kernel/webui/dashboard.html`** (`//go:embed`): the run-detail arc renderer
  - `arcDetail` gains an `llm.request` case → a compact summary line
    `N ctx chars · system …, user …` (falls back to `N msgs` for old events
    without the field).
  - `arcFull` gains an `llm.request` case → the expandable context-inspector
    block: `context: N chars across M message(s)` followed by a per-role
    breakdown (`system : N chars`, `user : N chars`, …).
  Reuses the existing expandable-row mechanism (▸/▾, M336); XSS-safe by
  construction (textContent / `el()` only — no HTML sink).

## Verification
- **Playwright, live daemon (the demo gate for UI):** brought up a daemon with
  `AGEZT_WEB_ADDR` + `AGEZT_SYSTEM_PROMPT="You are Agezt, a helpful autonomous
  agent."`, ran "what is 17 times 23", opened the web UI, clicked the run row,
  and saw the `llm.request` row render `61 ctx chars · system 42, user 19`;
  clicking it expanded to `context: 61 chars across 1 message(s) · system : 42
  chars · user : 19 chars` (42 = the system prompt length, 19 = the task).
- **Go lock-in** (`TestDashboard_RendersContextInspector`): asserts the embedded
  HTML contains the inspector wiring (`context_chars`, `context_by_role`,
  `ctx chars`, `llm.request`), so a refactor dropping it fails. The XSS-safe
  guard (`TestDashboard_NoUnsafeDOMSinks`) still passes over the new code.
- `go vet` clean; `GOOS=linux go build ./...` exit 0 (HTML embeds). Full suite
  **2132** passing (was 2131; +1), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible).

## Scope notes
- Closes the goal's priority-B context-inspector item at the run-detail level.
  A fuller "conversation surface" (per-turn message view) is a larger SPEC-07
  feature; the per-call context breakdown is the high-value, offline-verifiable
  slice and is now live.
- SPEC-10 §3 context *management* (compression/budgeting) remains the large
  open item, recorded in next.md.
