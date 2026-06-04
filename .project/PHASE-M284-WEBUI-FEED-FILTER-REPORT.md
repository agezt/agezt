# M284 — Web UI: filter the live event feed by kind

## Why
The dashboard's Event Feed is the full bus firehose — every event the daemon
emits, newest-first. On a busy daemon (multiple runs, schedules firing, tool
calls, budget/routing events per LLM round) the feed scrolls too fast to follow
one thing. An operator watching for, say, `provider.fallback` or `tool.*` had no
way to narrow it. This milestone adds a kind filter.

## What
- **`kernel/webui/dashboard.html`**:
  - A `#feedFilter` text input in the feed header (`filter kind…`), styled to sit
    to the right of the event count, before the `clear` button.
  - `feedFilter` (lowercased substring), `feedMatch(kind)`, and
    `applyFeedFilter()` which toggles `row.hidden` over the existing rows.
  - `addEvent` stamps each row with `data-kind` and hides it on arrival when it
    doesn't match the active filter — so the live stream respects the filter as
    events come in, not just on toggle.
  - The input's `input` listener updates `feedFilter` + re-applies. Filtering is
    pure client-side row toggling: switching or clearing is instant and never
    reconnects the SSE stream.

## Files
- `kernel/webui/dashboard.html` — filter input, CSS, `feedMatch`/`applyFeedFilter`,
  `addEvent` row tagging, boot wiring (edited).
- `kernel/webui/webui_test.go` — the dashboard-wiring test now guards
  `id="feedFilter"` + `function applyFeedFilter` (no new top-level test func).

## Verification
- `go test ./kernel/webui/` — green; full suite **1894**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven in a real browser** (Playwright) against a mock daemon with live
  runs:
  - Typing `task.` → of 12 feed rows, 4 visible (all `task.received` /
    `task.completed`), 8 hidden (`llm.response`, `budget.consumed`,
    `routing.decision`, `llm.request`).
  - With `tool.` active, a fresh run added 6 events → 0 visible, **no
    non-matching row leaked** (the live arrivals were filtered on arrival), and
    the input value persisted across the 10s periodic panel refresh.
  - Clearing the filter restored all 26 rows.

## Scope notes
- Pure front-end; no backend change, no new control-plane call, no dependency.
- Substring match on the full kind (not just a prefix) so `failed`, `tool`,
  `provider.` all work; case-insensitive.
- The feed remains the firehose — filtering only changes visibility, so toggling
  is free and history isn't lost. A future enhancement could show a
  "showing N of M" count or persist the last filter; deferred as unneeded.
