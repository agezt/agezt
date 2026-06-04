# M396 — Surface context.compacted in the web run-detail card (SPEC-10 §3 / SPEC-07)

## Context
M393–M395 made the agent loop manage its own context (compact oldest-first,
auto-size the budget, protect the grounding) and journal a `context.compacted`
event each time it trims. But the web run-detail card — which already renders the
tool-call I/O (M336/M341), isolation (M379), policy (M380), governor decisions
(M385), context size (M373) and artifacts (M392) — dropped `context.compacted`
to a bare kind line. The compaction was auditable in the journal but invisible in
the Live Monitor, so an operator couldn't see *why* a long run's context stopped
growing.

## What
- **`kernel/webui/dashboard.html`** — `arcDetail` gains a `context.compacted`
  case: a compact line `✂ compacted: elided N tool output(s), −R chars
  (BEFORE → AFTER) · budget B`. `arcFull` gains the matching expandable block with
  the full elided / reclaimed / before / after / budget breakdown. Both read only
  the journal payload keys the agent loop writes (`elided`, `reclaimed_chars`,
  `context_chars_before`, `context_chars_after`, `budget`) and render via the
  existing `el()`/`textContent` path — no new DOM sink.

## Verification
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersContextCompaction`:
  asserts the embedded HTML contains the case + every payload key it reads + the
  human label, so a rename on either side trips. `TestDashboard_NoUnsafeDOMSinks`
  still green (no `innerHTML`/`outerHTML`/`insertAdjacentHTML` introduced).
- **Negative control:** renaming `p.reclaimed_chars` → `p.reclaimed_DELETED` in
  the dashboard → the lock-in test FAILs on the `reclaimed_chars` marker;
  restored byte-identical.
- **Live Playwright demo** (mock provider, `AGEZT_DEMO_LOOP=1`,
  `AGEZT_CONTEXT_BUDGET=150`, fresh port 22417): a looping run produced repeated
  `context.compacted` events; the run-detail arc rendered
  `✂ compacted: elided 1 tool output(s), −56 chars (413 → 357) · budget 150`, and
  clicking the row expanded to the full `context compaction / elided: 1 / reclaimed:
  56 chars / before: 413 chars / after: 357 chars / budget: 150 chars` block.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2204** passing (was 2203; +1). CHANGELOG: not user-CLI-visible beyond the
  dashboard; recorded here (the run-detail surface is the user surface).

## Scope notes
- SPEC-10 §3 observability is now complete end-to-end: measured (M372), managed
  (M393–M395), and **surfaced** in both the journal and the Live Monitor (M396).
  Remaining §3 slice: LLM-summarise elided spans (replace the size stub with a
  model-written précis) — a larger feature needing a summarisation call.
