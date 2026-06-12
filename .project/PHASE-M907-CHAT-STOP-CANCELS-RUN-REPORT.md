# Phase M907 — Chat "Stop" actually cancels the server-side run

## The bug
The chat **Stop** button (and the implicit aborts when starting a new chat or
switching conversations mid-stream) only called `AbortController.abort()`, which
tears down the **browser's** SSE fetch. But the daemon's `cancel-on-disconnect`
is **off by default** (`AGEZT_CANCEL_ON_DISCONNECT`), and the chat runs through
the same governed `CmdRun` loop as `agt run`. So Stop left the agent loop running
**headless on the daemon** — continuing to call the model, run tools, and **spend
budget** — while the UI showed "stopped". A humane chat UI must actually stop.

## The fix — `frontend/src/lib/chatStore.tsx`
The backend already exposes the targeted cancel (`/api/cancel_run` →
`CmdCancelRun` by correlation id — the same one `agt` uses); only the wiring was
missing.
- `activeCorrRef` captures the streaming run's correlation id from the first
  frame that carries one (every forwarded event has `correlation_id`).
- `abortActiveRun()` aborts the fetch **and** best-effort `POST /api/cancel_run
  {correlation}` so the daemon cancels the run. Failures are swallowed (a missed
  cancel must not throw into the UI).
- `stop()`, `newChat()`, `selectConversation()`, `removeConversation()` (the
  abort-while-busy paths) now route through `abortActiveRun()`. The ref is reset
  at the start and in the `finally` of each stream.

No behavior change when a run finishes normally or when navigating away via the
router (the provider lives above the router, so unmount still doesn't abort — only
the explicit affordances do, and now they cancel cleanly).

## Tests — `frontend/src/lib/chatStore.cancel.test.tsx`
Renders `ChatProvider` with a fake `streamRun` (hands back the frame callback,
resolves only on abort) and mocked `postAction`:
- after a frame carrying `correlation_id: run-42`, clicking Stop posts
  `/api/cancel_run {correlation: "run-42"}`;
- stopping **before** any correlation id was seen cancels the stream (returns to
  idle) without calling `/api/cancel_run`.

## Gate
`tsc --noEmit` ✓ · full vitest **528 pass** (77 files) ✓ · `vite build` → embedded
`kernel/webui/dist` rebuilt (LF) ✓ · `go build ./...` + `kernel/webui` test green
with the new dist ✓ · go.mod unchanged · no Go source changed (frontend + dist
only).

## Notes
Pure product-layer reliability fix (the standing "humane chat UI" priority) with a
direct cost impact — a stopped chat no longer silently burns tokens. Composes with
the existing `continueRun`/`retry` affordances.
