# Phase M908 — "Cancel" button in the live-run cockpit

## Gap
`SteerControls` (the in-flight-run cockpit in `RunDetail.tsx`, M608) let an
operator **pause / resume / step / steer** a live run — but not **stop** it. The
only kill switch in the UI was the **global** Halt, which stops *everything*. The
targeted cancel (`/api/cancel_run` → `CmdCancelRun` by correlation id — kills one
run, leaves the kernel and other runs untouched) existed on the backend and in
`agt`, but had no button. This is the "stop" leg of the overseer's
supervise/intervene/**stop**/modify mandate (#46).

## What shipped — `frontend/src/components/RunDetail.tsx`
- A **Cancel** button in the cockpit's control row (right-aligned, `bad`-styled).
- **Two-step confirm**, self-contained so `SteerControls` stays a pure prop-driven
  component (no `useUI`/provider coupling): first click arms ("Confirm cancel"),
  second click fires `act("/api/cancel_run")` (which posts the run's correlation
  id). `onBlur` disarms, so a stray first click doesn't linger.
- Reuses the existing `act()` helper (shared busy/error state), so a failed cancel
  surfaces in the same inline error line as pause/steer.

## Tests — `frontend/src/components/RunDetail.steer.test.tsx`
- First click arms the confirm and issues **no** request; second click posts
  `/api/cancel_run {correlation: "run-7"}`.
- Cancelling issues exactly one action (it doesn't also pause/steer).

## Gate
`tsc` ✓ · full vitest **530 pass** (78 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only.

## Notes
Pairs with M907 (chat Stop cancels the run): both surfaces now expose the targeted,
budget-saving cancel. The run's own event arc still drives the paused/live state,
so the cockpit stays in sync with the daemon regardless of who issued the cancel.
