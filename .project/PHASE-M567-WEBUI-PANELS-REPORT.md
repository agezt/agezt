# Phase M567 — Web UI: port the remaining panels to bespoke React views

**Type:** Feature (completes the M566 React migration)
**Date:** 2026-06-07
**Branch:** `feat-webui-panels`

## Goal

After M566 shipped the React SPA shell + flagship views (feed, Status, Runs,
Budget, Flow Studio), the remaining ~12 read panels rendered through a generic
JSON fallback. This phase ports them to first-class React views, completing the
Web-UI migration. Pure-frontend: every `/api/*` route already exists, so the Go
side is untouched — only the rebuilt bundle changes.

## What shipped (`frontend/src/`)

- **Bespoke views** (field shapes ported faithfully from the old dashboard
  renderers): `Config` (model/tools/plugins/ask-policy + env-presence chips +
  paths + routing), `Cache`, `Providers`, `Tools`, `Policy`, `Schedules`,
  `World`, `Skills`, `Standing`, `Memory`, `Inbox`, `Reflect`, `Approvals`.
- **Log drill-downs** via a shared `LogDetail` (lazy-fetch + toggle): Providers →
  `/api/provider_log`, Tools → `/api/tool_log`, Policy → `/api/policy_log`.
- **Actions** via a shared `ActionButton` (query-arg POST + reload): Skills
  promote/quarantine/revert, Memory forget, World forget, Approvals approve/deny.
- **World graph**: a React Flow node-link diagram (`WorldGraph`, circular layout)
  of the world model's entities + relations.
- **Shared scaffolding**: `components/Panel` (render-prop shell: fetch + refresh +
  loading/error + reload), `Stats`/`Row`/`Count` primitives, `lib/format`
  (money/pct/sort). Removed the `GenericPanel` fallback; `App` nav now maps every
  view to its bespoke component.

## Verification

- **Build:** `npm run build` (tsc typecheck + Vite) clean; `dist` rebuilt +
  byte-reproducible run-to-run.
- **Go:** `kernel/webui` tests pass (unchanged); full suite green
  (`GOMAXPROCS=3 go test ./... -p 2`, 75 pkgs). No Go files changed;
  `go.mod`/`go.sum` unchanged.
- **Real browser (Playwright):** navigated the console, clicked Providers (+
  expanded the routing-log drill-down, which lazy-fetched `/api/provider_log`) and
  World — **0 console errors** throughout; panels render correctly under the
  strict CSP.

## Counts

- Packages 75; Go tests 2425 (unchanged — pure-frontend change).
- No Go dependency added.

## Follow-ups (optional)

- Run-detail isolation/policy/context cards inside the Runs event-arc (the old
  dashboard's per-event detail rendering).
- Vitest/Playwright component tests for the React side in CI.
