# Phase M913 — Header approvals bell + de-staled "Needs attention"

## Owner asks
1. "Approvals'da bekleyen bir şeyi webui'da görme ihtimalim az" + "header'da bir
   notification, bekleyen iş tadında alan lazım ki onaylayabileyim ya da en
   azından sayfasına gidebileyim."
2. "Overview'da çok eskide kalan **Needs attention (4)** — daemon halted vs
   error'lar duruyor, neden yıllarca duracak gibi bunlar?"

## 1. Approvals header bell — `frontend/src/components/ApprovalsBell.tsx`
A global pending-approval indicator in the header (next to the AlertBell), on
**every** view:
- Counts pending HITL requests from `/api/approvals`, **live** — refetches on any
  `approval.*` event, plus a 15s safety poll.
- Badges the header (amber, pulsing) with the pending count.
- Click → a dropdown listing each pending request (`capability — reason`) with
  **Approve / Deny** buttons wired to `/api/decide` (`grant`/`deny`), plus an
  "Open" link to the full Approvals page. With nothing pending, the bell just
  jumps to Approvals.
- Closes on outside-click; optimistic removal on decide, reconciled by a refetch.

So a gated capability waiting on the operator is now visible and actionable from
anywhere, not only if you happen to open the Approvals tab.

## 2. De-stale "Needs attention" — `frontend/src/lib/alerts.ts`
The Dashboard pulls alerts from `/api/journal` (300 events) and the nav badge from
the live buffer, both of which **backfill weeks of history** — so an old halt or a
run that failed days ago sat in "Needs attention" forever. Now:
- **`daemonHalted(events)`** — the kernel is "halted" only if the latest
  halt/resume transition is a halt; a "daemon halted" alert a later `resume`
  already cleared is dropped.
- **Recency window** — `recentAttentionAlerts` / `attentionAlertCount` take an
  optional `nowMs` (+ `windowMs`, default 24h); alerts older than the window age
  out of the count and the strip. Callers (`Dashboard`, the nav badge in `App`)
  now pass `nowMs: Date.now()`.
- `attentionAlertCount` also dedupes by id now, so the badge and the cockpit strip
  always agree.
- Backward compatible: `recentAttentionAlerts(events, 5)` (bare-number limit) and
  `attentionAlertCount(events)` (no window) still behave as before for any other
  caller/test.

## Tests
- `alerts.test.ts` — `daemonHalted` transitions; a resumed halt is dropped from
  both the list and the count; a still-in-effect halt is kept; alerts past the
  recency window age out only when `nowMs` is supplied (legacy path unchanged).
- `ApprovalsBell.test.tsx` — `approvalLabel` field fallbacks.

## Gate
`tsc` ✓ · full vitest **542 pass** (80 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only.

## Process note
Built in an isolated git worktree (`AGEZT-attention`, branch
`feat/m913-attention-approvals`) from `origin/main` per the cross-session recipe,
so the dist is clean by construction (no foreign uncommitted code baked in). M912
(MCP catalog, PR #339) is still in flight from another session — both touch
CHANGELOG.md + the dist, so whoever merges second rebases + rebuilds.
