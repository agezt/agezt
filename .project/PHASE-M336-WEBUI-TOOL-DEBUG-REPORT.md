# M336 — Web UI run-detail full tool I/O view (SPEC-07 tool-call debug)

## Why
Continuing the v1.0-conformance goal. SPEC-07 (web monitoring surface) calls for an
operator to be able to *debug a run* — to see exactly what a tool was invoked with
and what it returned. The run-detail modal already listed the journalled event arc,
but tool rows only showed a one-line preview (`shell {"command":"dir"}` and a
clipped result string). The full tool input/output — which **is** journalled
(`tool.invoked.input`, `tool.result.output`) — was not reachable from the UI, so an
operator had to drop to `agt journal` / raw JSONL to inspect a tool call. This is
the highest-value locally-verifiable SPEC-07 gap (assistant message *text* is not
journalled by design, so it genuinely cannot be surfaced here — but tool I/O can).

## What
- **`kernel/webui/dashboard.html`** (embedded via `//go:embed`):
  - `arcFull(ev)` helper: for `tool.invoked` returns `"input:\n"` + pretty-printed
    (`JSON.stringify(…, null, 2)`) tool input; for `tool.result` returns
    `"output:\n"` (or `"error:\n"`) + the full result string. Other event kinds
    return `""` (not expandable).
  - The `openRun` arc loop now renders a ▸/▾ toggle on tool rows. A tool row is
    marked `.expandable` and, on click, toggles a `<pre class="toolio">` block
    holding the full untruncated I/O (scrollable, `max-height:260px`,
    `white-space:pre-wrap`). Non-tool rows are unchanged.
  - CSS: `.arc .ev.expandable{cursor:pointer}`, `.arc .ev .tog`, and the `.toolio`
    block styling.

## Verification
- **Live Playwright** against a real daemon (`AGEZT_PROVIDER=mock`,
  `AGEZT_WEB_ADDR=127.0.0.1:8771`): ran `agt run "list the files in the current
  directory"` → a completed run with `tool.invoked` (seq 6) and `tool.result`
  (seq 9). Opened the run-detail modal, confirmed both tool rows carry the ▸
  toggle while non-tool rows do not. Clicked `tool.invoked` → toggle flips ▸→▾ and
  a `.toolio.show` block renders the full pretty-printed input
  (`input:\n{\n  "command": "dir"\n}`). Clicked `tool.result` → a second
  `.toolio.show` block renders the **full 1536-char** `dir` output, untruncated
  (the row preview was clipped to "…Directo"; the expanded block ends with
  "…3.518.226.784.256 bytes free"). Only console error is a harmless favicon 401.
- `kernel/webui` tests pass; `go vet ./kernel/webui/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2044** passing (`go test ./...` exit 0). `go.mod` /
  `go.sum` unchanged. UI-only change (no new Go test; verified via live Playwright).

## Scope notes
- Assistant message **text** is deliberately not journalled (only `text_chars` +
  previews), so the modal still cannot show full assistant prose — that is a
  by-design journal constraint, not a UI gap. Tool I/O **is** journalled and is now
  fully inspectable, which is the actionable half of run debugging.
