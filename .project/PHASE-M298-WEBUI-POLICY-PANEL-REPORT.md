# M298 — Web UI: Policy panel + decision-log modal

## Why
The Web UI had no view of the Edict policy engine — the security/governance layer
that allows or denies every capability the agent uses. The CLI has
`agt edict stats` / `agt edict log`, but an operator watching the dashboard
couldn't see what the policy engine was doing. This closes that CLI↔Web-UI gap
with an always-populated governance view (a policy decision fires on every tool
invocation).

## What
Mirrors the Providers (M285+M286) / Tools (M287+M288) aggregate+drill pattern.

- **`kernel/webui/webui.go`**: `"/api/policy" → CmdEdictStats` (apiRoutes,
  parameterless read); `"/api/policy_log" → CmdEdictLog` (readArgsRoutes,
  forwarding `limit`, `denied`).
- **`kernel/webui/dashboard.html`**:
  - A `Policy` panel + `policy` renderer: `allowed` / `denied` (with hard count) /
    `denial rate`, a clickable `N decision(s)` line, and a red
    `denied by capability` breakdown.
  - `openPolicyLog()` modal: each decision `allow | DENY | DENY(hard)
    <capability> <tool>` (deny in red) + reason + local time, newest-first, via
    `/api/policy_log`.
  - `liveRefresh` routes `policy.*` events to the panel; `policy` in `PANELS`.

## Files
- `kernel/webui/webui.go` — `/api/policy` + `/api/policy_log` routes (edited).
- `kernel/webui/dashboard.html` — panel, renderer, `openPolicyLog`, liveRefresh,
  PANELS (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestPolicyRouteProxiesEdictStats`, `TestPolicyLogRouteForwardsLimit`.
  - `edict_stats` added to the `TestAPIReadOnly` allowlist; Policy panel + modal
    guards.

## Verification
- `go test ./kernel/webui/` — green; full suite **1909**, 68 packages, `go test
  ./...` exit 0; `gofmt -l` clean; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon driving the `file` tool → `GET /api/policy`
  returned `total:2 allowed:2 denied:0`; `GET /api/policy_log` returned the two
  `allow file.write file` decisions with their reasons.
- **Live-proven in a real browser** (Playwright): the Policy panel rendered
  `allowed 2 / denied 0 / denial rate 0% / 2 decision(s) · click for log`;
  clicking opened the "Policy decision log" modal listing both decisions
  (`allow file.write file` + `level L2; AskPolicy=AskAllow …`). The DENY rendering
  path is the same row with a red `DENY`/`DENY(hard)` verdict.

## Scope notes
- Read-only over the existing `edict_stats` / `edict_log` commands; no new endpoint
  logic, no new event, no dependency.
- Completes the governance/security observability on the Web UI, mirroring the CLI
  edict triad (show/log/stats). The Web UI panel set now covers run health
  (Stats), spend (Budget), cache (Cache), routing (Providers), execution (Tools),
  and policy (Policy) — each with aggregate + drill where applicable.
