# M277 — Web UI: click a run for its event arc

## Why
The Runs panel (M275) showed each run's outcome, but not *what the run did*. The
most interactive missing piece was a drill-down: click a run → see its full
journaled arc (the `agt runs show` story — tools invoked, results, budget, final
answer). This makes the Web UI a real operational console, not just a status
board.

## What
- **`kernel/webui/webui.go`** — a new route *type* for read-only commands that
  take query arguments:
  - `readArgsRoutes` maps `/api/journal` → `CmdJournalGrep`, forwarding only the
    allowlisted `correlation_id` / `kind` / `limit` args.
  - `readArgsProxy` serves them over GET (the command is read-only), copying just
    the allowlisted args — the browser can't pass arbitrary parameters.
- **`kernel/webui/dashboard.html`**:
  - Each Runs row is now `click`able and calls `openRun(r)`.
  - A detail **modal** (overlay + sticky header + scroll body) fetches the run's
    events via `/api/journal?correlation_id=…` and renders the arc: per event a
    seq + coloured kind + a best-effort one-line detail (`arcDetail`) — tool name
    and input for `tool.invoked`, ✓/✗ and output for `tool.result`, model + cost
    for `budget.consumed`, the answer for `task.completed`, the reason for
    `task.failed`. Closes on ✕, Esc, or a click on the backdrop.
  - Modal CSS (overlay, monospace arc rows).

## Files
- `kernel/webui/webui.go` — `readArgsRoutes`, `readArgsProxy`, registration
  (edited).
- `kernel/webui/dashboard.html` — clickable runs, run-detail modal, `arcDetail`,
  modal CSS (edited).
- `kernel/webui/webui_test.go` — `TestJournalRouteForwardsCorrelationOnly` (new):
  the route issues `journal_grep`, forwards `correlation_id`, and drops a
  non-allowlisted param; the dashboard-wiring test now guards `openRun` +
  `/api/journal`.

## Verification
- `go test ./kernel/webui/` — green; full suite **1882 → 1883** (+1), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on the Go files; `go vet ./kernel/webui/`
  clean; `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven in a real browser**: ran a tool-using task (default mock →
  shell `dir` + answer), clicked the run in the Runs panel, and the modal showed
  the full 11-event arc — `task.received → llm.request → budget.consumed →
  policy.decision → tool.invoked shell {"command":"dir"} → tool.result shell ✓ <dir
  listing> → llm.request → budget.consumed → llm.response 377 chars →
  task.completed "<answer>"`. Esc/close worked.

## Scope notes
- The arg-allowlisted GET read route is a small, safe generalisation (read-only,
  fixed arg set) that future detail views (e.g. filter the feed by kind) can
  reuse.
- Reuses `CmdJournalGrep` — same data `agt journal grep --correlation` returns —
  so the Web UI and CLI stay consistent.
- Demo artifacts (`webui-*.png`) remain gitignored.
