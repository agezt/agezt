# PHASE M869 — Active-run cards name the responsible agent

**Status:** shipped
**Milestone:** M869 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Product/live-monitoring layer — answers the core supervisory question
"who is doing what right now" by labelling each active run on the Overseer with
the agent running it.

## What shipped

`frontend/src/views/Overseer.tsx`: active-run cards now carry an agent chip.

- A memoized `corrAgent` map (`correlation_id → agent slug`) is learned from the
  live event stream — `task.received` events carry the agent in `actor`. First
  writer wins per correlation.
- Each active-run card renders a small accent chip (`UserRound` icon + slug) when
  the agent is known. Runs that started before the page loaded have no `actor` in
  the buffer, so the chip is simply omitted — graceful, consistent with the rest
  of the live UI (same cold-start semantics as the badges).

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/views/Overseer.tsx` and the
rebuilt `kernel/webui/dist`. Reuses the already-subscribed `useEvents()` feed —
no new route, no Go change. The concurrent session's in-flight kernel edits are
untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vite build` emits the committed-LF
  dist (1855 modules).
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped (it would compile the concurrent in-progress Go edits).

## Notes
- Completes the supervisory "what / who / outcome" picture on the Overseer:
  active runs now show intent + **agent** + model + iters (M862/M869), a live
  activity ticker shows what just happened (M867), and the nav badge shows the
  in-flight count from anywhere (M868).
