# M293 — Prompt-cache savings aggregate + Web UI Cache panel

## Why
The cost arc (M289–M291) made billing cache-aware and M292 surfaced the daily
spend, but the *payoff* of caching — how many tokens came from cache and how much
that saved — was nowhere visible. This adds the aggregate and shows it.

## What
- **`kernel/controlplane/cache_stats.go`** (new): `CmdCacheStats` /
  `handleCacheStats` folds `budget.consumed` events into `cached_input_tokens`,
  `cache_write_input_tokens`, `saved_microcents`, and `calls`. Savings per call =
  the no-cache baseline `governor.CostMicrocents(model, input, output)` (every
  input token at the full input rate) minus the event's recorded
  `cost_microcents` (already cache-discounted), floored at zero. Tenant-scoped
  (`kernelFor(tenantOf(req))`), optional `since_ms` window, registered in the
  dispatch switch and the tenant read-only allowlist.
- **`kernel/webui/webui.go`**: `"/api/cache" → CmdCacheStats` apiRoute.
- **`kernel/webui/dashboard.html`**: a Cache panel + `cache` renderer
  ($ saved, cache-read tokens, cache-write tokens, priced calls; an empty-state
  line); `liveRefresh` refreshes Budget + Cache together on `budget.*`; `cache`
  added to the initial-load `PANELS`.

## Files
- `kernel/controlplane/cache_stats.go` (new), `protocol.go` (CmdCacheStats),
  `server.go` (dispatch), `tenant.go` (allowlist) (edited).
- `kernel/webui/webui.go`, `kernel/webui/dashboard.html` (edited).
- `kernel/controlplane/cache_stats_test.go` (new) `TestCacheStats_Aggregates`;
  `kernel/webui/webui_test.go` (new) `TestCacheRouteProxiesCacheStats` +
  `cache_stats` in the read-only allowlist + Cache panel guard.

## Verification
- Full suite **1904**, 68 packages, `go test ./...` exit 0; `go vet` clean on the
  touched packages; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.
  (`gofmt -l` flags `tenant.go` — a pre-existing CRLF artifact; `git diff` shows
  only my one-line allowlist addition, which is gofmt-valid.)
- **Live-proven end-to-end**: `AGEZT_DEMO_CACHED=1` (10000 input / 9000 cache-read
  / 500 cache-write / 200 output, sonnet) → `GET /api/cache` returned
  `saved_microcents:2392500 cached_input_tokens:9000 cache_write_input_tokens:500
  calls:1` — baseline 3_300_000 − recorded 907_500 = 2_392_500 ($0.0024).
- **Live-proven in a real browser** (Playwright): the Cache panel rendered
  `saved $0.0024 / cache reads 9000 tok / cache writes 500 tok / priced calls 1`.

## Scope notes
- Reuses the exported `governor.CostMicrocents` for the no-cache baseline — no new
  pricing logic. Read-only journal fold, same shape as the other `*_stats`
  commands.
- The Cache panel is the visible capstone of the cost arc (M289 cache-aware
  billing → M290 Anthropic wiring → M291 cache-write premium → M292 Budget panel →
  M293 savings). A future `agt cache` CLI could reuse `CmdCacheStats` for full
  CLI↔Web-UI parity.
