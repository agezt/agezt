# PHASE M877 — Per-turn timestamps in the chat

**Status:** shipped
**Milestone:** M877 (numbered ABOVE the concurrent session's planned M868–M876
hermes arc to avoid a milestone-number collision — per the standing "check git
log first" note; main currently tops out at M869, all mine).
**Theme:** Product/humane-chat layer — the one clear missing staple: each chat
exchange now shows *when* it happened.

## What shipped

Chat turns now carry and display a creation timestamp:

- `chat.ts` — `ChatTurn` gains an optional `ts?: number` (wall-clock ms). The pure
  reducer `newTurn()` stays time-free, so its unit tests are unchanged.
- `chatStore.tsx` — a small `freshTurn()` helper (`{ ...newTurn(), ts: Date.now() }`)
  replaces `newTurn()` at the four message-creation sites (send, retry,
  continueRun, editAndResend), so only turns the store creates on a real send get
  stamped.
- `Chat.tsx` `TurnMeta` — appends `fmtTime(turn.ts)` to the meta line
  (`as <agent> · <model> · <n> iters · <cost> · <time>`). Older persisted turns
  lack `ts` and simply omit it — graceful, no migration.

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/lib/chat.ts`,
`frontend/src/lib/chatStore.tsx`, `frontend/src/views/Chat.tsx`, and the rebuilt
`kernel/webui/dist`. No Go change; the concurrent session's kernel edits are
untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; the full chat suite green —
  `vitest run src/lib/chat src/lib/chatStore src/views/Chat` = **42/42** (the new
  optional field doesn't disturb any deep-equal/fold assertion). `vite build`
  emits the committed-LF dist (1855 modules).
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped.

## Notes
- The timestamp is the turn's creation (≈ when the intent was sent), serialized
  with the conversation so it survives reload. Persistence is forward-compatible:
  it's an additive optional field.
- **Milestone numbering:** the concurrent session's `hermes-gap-arc` memory now
  claims an M868–M876 arc, which overlaps my already-merged M868/M869. I jumped to
  M877 to stop contributing to the collision; the overlap on M868/M869 will need
  reconciling when their work lands (their commits aren't on main yet).
