# M275 — Web UI: a live Runs panel

## Why
The user asked to actually *see* the product running — so we brought up the
daemon + Web UI and exercised it in a browser. The single most glaring gap was
that the live monitor had no **Runs** panel: it streamed raw events but never
showed the run list with outcomes (the `agt runs list` view — the most useful
operational surface, and the payoff of the whole runs-observability arc M28–M60).
This milestone adds it, and makes the read panels refresh on the live event
stream instead of only on a timer.

## What
- **`kernel/webui/webui.go`** — added `"/api/runs": controlplane.CmdRunsList` to
  the read-only `apiRoutes` (proxied with default limit; no args needed).
- **`kernel/webui/dashboard.html`**:
  - A **Runs** panel section, placed right after Status.
  - A `runs` renderer: per-run `statusChip` (completed→green, failed/abandoned→
    red, running→accent via new `.chip.s-*` CSS), the intent, a `↳` marker for
    sub-agent runs, and `duration · iters · $spend`.
  - `statusChip(status)` helper + the status colour classes.
  - **Event-driven live refresh** (`liveRefresh`): `addEvent` now debounces a
    panel reload keyed by the streamed event kind — `task.*`→Runs, `skill.*`→
    Skills, `memory.*`→Memory, `world.*`→World, approval events→Approvals — so a
    finishing run (or a changing skill) shows up immediately, not at the next
    10s tick. `runs` was also added to the initial load + periodic refresh sets.

## Files
- `kernel/webui/webui.go` — `/api/runs` route (edited).
- `kernel/webui/dashboard.html` — Runs panel, renderer, status chips, live
  refresh (edited).
- `kernel/webui/webui_test.go` — `TestRunsRouteProxiesRunsList` (new); the
  read-only allowlist gained `runs_list`; `TestDashboardServedAtRoot` now guards
  the Runs panel wiring.

## Verification
- `go test ./kernel/webui/` — green; full suite **1880 → 1881** (+1), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on the Go files; `go vet ./kernel/webui/`
  clean; `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged (still
  stdlib-only).
- **Live-proven in a real browser** (Playwright against the running daemon):
  - the Runs panel rendered 3 seeded runs with green `completed` chips,
    intents, and `Nms · 1 it`;
  - a 4th run triggered from the CLI **while the page was open** appeared at the
    top of the panel automatically (the `task.completed` event drove the
    refresh), confirming the event-driven update — no manual reload.
  - screenshot captured (gitignored demo artifact).

## Scope notes
- No new dependency, no new control-plane command — the panel reuses the existing
  `CmdRunsList` the CLI already uses, keeping the Web UI and `agt runs` exactly
  consistent (SPEC-07 §0: one event truth, many views).
- Demo artifacts (`webui-*.png`, `.playwright-mcp/`) are gitignored, not
  committed.
