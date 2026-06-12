# Phase M919 — Proactive desktop notifications

## Ask
The owner picked "proaktif ulaşma" (proactive reach-out) as a direction. The pain
they named: a pending approval (or a failure) is easy to miss because it only
shows if you're looking at the right tab. The header ApprovalsBell (M913) helps,
but still needs you on the page. **AGEZT should reach OUT.** First, conflict-free
step: browser/desktop notifications that fire even when the tab is in the
background.

## What shipped (frontend only)
### `frontend/src/lib/notify.ts`
- **Opt-in pref** (`notifyEnabled`/`setNotifyEnabled`, localStorage, off by
  default — browser notifications need an explicit user gesture).
- **`notifyEventClassify(event)`** — pure: maps an event to a `DesktopNotice`
  (`title`, `body`, `tag` for coalescing, `hash` = view to open) for the **narrow
  high-signal set** only — `approval.requested`, `task.failed`, `halt`,
  `budget.exceeded` — and `null` for the firehose. No interrupting on routine
  events.
- `notifySupported`/`notifyPermission` wrap the Notification API.

### `frontend/src/components/NotifyToggle.tsx`
- A **header toggle** (bell icon: off / ringing-on / blocked). Enabling requests
  Notification permission (the required user gesture); a toast confirms.
- **`useDesktopNotifications(enabled)`** — subscribes to the **live** event stream
  (no journal backfill, so no replay-spam on load) and fires a `Notification` for
  each high-signal event while enabled + granted. Clicking the notification
  focuses the window and routes to the relevant view; the `tag` coalesces repeats.

Wired into `App.tsx` beside the Approvals/Alert bells.

## Why this scope
The SSE `subscribe` only delivers events from connect-time on (verified — the
backfill lives in `/api/journal`, a separate fetch), so notifications never replay
history. Off-by-default + explicit permission keeps it from nagging. Channel-based
push (Telegram/desktop daemon) is the natural backend follow-up.

## Tests — `frontend/src/lib/notify.test.ts`
`notifyEventClassify` (the four high-signal kinds → correct title/tag/hash; routine
events → null; sparse-payload fallbacks) and the pref round-trip. 4 tests; full
suite **556 pass**.

## Gate
`tsc` ✓ · full vitest **556 pass** (81 files) ✓ · `vite build` → embedded dist (LF)
✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend + dist
only.

## Follow-ups (same "proactive" direction)
- **Daemon-side push** to an already-connected channel (Telegram/Slack/Discord) so
  you're pinged even with no browser open — the bigger win, but backend.
- Per-category toggles (approvals vs failures vs alerts) if the single switch
  proves too coarse.
