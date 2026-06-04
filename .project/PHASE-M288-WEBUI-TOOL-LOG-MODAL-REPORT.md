# M288 — Web UI: Tools panel → per-call invocation log

## Why
M287 added a Tools panel showing the *aggregate* execution picture (calls, error
rate, per-tool latency). Its natural companion — the one the Providers panel got
in M286 — is the per-call *timeline*: what tool each call ran, with what input,
what came back, ok or error, and how long it took. The control plane already
serves this (`CmdToolLog`, M66); this milestone surfaces it, making the two
execution surfaces (Providers, Tools) symmetric (aggregate + drill-down).

## What
- **`kernel/webui/webui.go`**: a new read-only args route
  `"/api/tool_log" → controlplane.CmdToolLog` (forwarding `limit`, `tool`,
  `errors`), in `readArgsRoutes` (GET, read-only).
- **`kernel/webui/dashboard.html`**:
  - `openToolLog()` reuses the modal shell + `/api/tool_log?limit=40`, rendering
    each invocation newest-first: `<tool> ✓|✗ (<latency>)` (✗ in red) with the
    `input → output` preview (each capped at 100 chars) and a local timestamp.
  - The Tools panel's per-tool rows and the count line are now `item click` →
    `openToolLog`, with a "· click for invocation log" hint.

## Files
- `kernel/webui/webui.go` — `/api/tool_log` route (edited).
- `kernel/webui/dashboard.html` — `openToolLog`, clickable rows + hint (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestToolLogRouteForwardsLimit` (route forwards `limit`, drops a stray
    arg).
  - the dashboard-wiring test now guards `function openToolLog` + `/api/tool_log`.

## Verification
- `go test ./kernel/webui/` — green; full suite **1898**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_FILE_EDIT=1` drove the
  `file` tool → `GET /api/tool_log` returned 2 invocations with input/output,
  `error:false`, `duration_ms:1`.
- **Live-proven in a real browser** (Playwright): clicking a Tools row opened the
  "Tool invocation log" modal listing 2 calls newest-first —
  `file ✓ (1ms)` with `{"op":"replace",…} → replaced 1 occurrence(s)…` and
  `{"op":"write",…} → wrote 30 bytes to notes.txt`.

## Scope notes
- Read-only over an existing control-plane command; no new endpoint logic, no new
  event, no dependency.
- Completes the execution-observability pairing: Providers aggregate (M285) +
  routing-log (M286); Tools aggregate (M287) + invocation-log (M288). All four
  feed off the same journal events the CLI's `agt provider …` / `agt tool …`
  commands read.
- **Flagged for a future focused frontier (not webui): cache-aware cost
  accounting.** The catalog already carries `cost.cache_read` / `cost.cache_write`
  (`kernel/catalog/types.go`), but `governor.modelPrice` only has input/output
  rates, `agent.Usage` has no cached-token field, and the openai provider doesn't
  parse `prompt_tokens_details.cached_tokens` — so prompt-cached calls on real
  reasoning models (e.g. the gpt-5.5 gateway) are billed at the full input rate
  (cost over-estimate). A proper fix spans `agent.Usage` (+CachedInputTokens),
  the provider parse, `governor` cost math (+CacheRead rate from catalog), and a
  mock usage hook for the demo. Self-contained but multi-file; deserves its own
  milestone.
