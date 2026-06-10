# Phase M791 — console catch-up: Standing agent field + Board threading

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 9
(the console renders what M788/M790 built).

## What

- **Standing view**: *runs as <slug>* chip on agent-bound orders; **Run as
  agent** picker (reused AgentPicker) in both New and Edit forms — the
  autonomous-answerer recipe is now assemblable entirely from the UI. Edit
  posts `agent` (present-and-empty clears, matching the M790 server).
- **Agent board**: *from → to* chips on addressed messages, a *reply* marker
  on answers, and an **awaiting reply** badge on unanswered questions —
  computed over the WHOLE board (topic filters can't make it lie). The board
  read handler now carries id/to/reply_to (Go).

## Honest find

The first `awaitingReply` flagged REPLIES as awaiting (a reply is addressed
too, and nobody replies to a reply). The component test caught it (two badges
where one belonged); fixed: a message with `reply_to` never awaits.

## Tests (4 new; 457 vitest total, 69 files)

`awaitingReply` matrix (unanswered flagged / answered cleared / replies never
/ broadcasts never) · Board renders chips + reply marker + exactly one badge
from a mocked feed · Standing edit posts `agent` (existing M729 test extended
— present-and-empty clears).

## Browser verification (isolated daemon, new dist)

Agent + agent-bound standing order seeded via CLI → #standing showed the
*runs as researcher* chip → New-order form's picker listed and selected the
agent (trigger showed the slug). 0 console errors; clean shutdown; 0 panics.

## Gate

Full Go suite + vet + staticcheck green; 457 vitest; dist rebuilt (LF) and
committed; go.mod unchanged. CI org-billing still blocked → local battery +
arc-authority merge.

## Next in the arc

workdir wiring · per-agent daily budget ledger.
