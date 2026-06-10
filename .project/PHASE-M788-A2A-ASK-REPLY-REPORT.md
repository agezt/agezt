# Phase M788 — A2A ask/reply: addressed agent-to-agent messages

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 6
(gap #2 of the vision gap analysis — direct agent-to-agent messaging).

## What

The shared board (M647) grows an addressed layer: send (to=slug, returns id) ·
inbox (unanswered-first; all=true includes answered) · reply (id → linked,
addressed back to the asker) · replies (the asker reads answers). Addressed
messages journal `board.posted` under **`board.dm.<recipient>`**, so a standing
order with that event trigger wakes exactly the agent being asked (M656 wake +
M783 identity = ask → wake → answer → read).

## Changes

- `kernel/board`: Message += ID/To/ReplyTo (additive, old board.json loads);
  `Send` (id-assigning generalisation; Post now wraps it) + `Get`/`Inbox`
  (case-insensitive, unanswered filter)/`Replies` (conversation order).
- `plugins/tools/boardtool`: ops send/inbox/reply/replies; `PostNotifier`
  re-signed to carry the full Message (breaking change, both callers in-PR);
  msgView += id/to/reply_to; description teaches the ask/reply flow + the
  board.dm.<slug> wake trigger.
- `cmd/agezt`: notifier publishes `board.dm.<recipient>` for addressed
  messages, payload += id/to/reply_to.

## Tests (4 new/extended)

- kernel/board: round-trip + persistence (ids, case-insensitive inbox,
  broadcast never in inboxes, reply clears unanswered, includeAnswered,
  Replies linkage, reopen).
- boardtool: `TestAskReplyRoundTrip` (send→inbox→reply→inbox-clears→replies,
  addressed back to asker, send AND reply notify); bad-input table (no
  to/text/id, ghost-id reply); notifier re-sign covered by updated
  `TestPost_NotifiesWithCorrelation`; read/inbox/replies never notify.

## Gate

Full suite `-p 2 ./...` green; vet + staticcheck clean; linux cross-build OK;
go.mod unchanged; no frontend change (Board view renders new fields via the
same JSON). CI org-billing still blocked → local battery + arc-authority merge.

## Next in the arc

Chat: converse AS a named agent · workdir wiring · per-agent daily budget
ledger · Board view: render to/reply_to threading.
