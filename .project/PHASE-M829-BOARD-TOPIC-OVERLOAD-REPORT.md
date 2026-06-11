# Phase M829 — Agent Board: tame the topic overload

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "agent board da
milyon topic olacak bu gidişle izlenemez hale gelecek, onu bir adam et."

## Why

The board rendered EVERY topic as a filter chip in a flat wrap. With many agents
the topic list grows without bound and the chip row swallows the whole view.

## What changed (`frontend/src/views/Board.tsx`)

- **Topic search:** when there are >12 topics, a filter box appears
  (`filter N topics…`) that narrows the chips by name.
- **Visible cap + scroll:** at most 24 chips show (highest-message-count first);
  the chip row is height-capped and scrolls. A **`+N more`** chip expands to all;
  **show fewer** collapses back.
- **Selected stays visible:** the currently-selected topic is always kept in the
  visible set even if the cap/filter would hide it, so the filter never hides what
  you're looking at.

The message list (already limit-200 from the fetch) and per-topic filtering are
unchanged.

## Tests

- vitest `Board.test.tsx`: with 30 topics the search box appears, only the first
  24 chips render + `+6 more`, a tail topic is capped out, and typing it in the
  filter surfaces it (while a capped-in topic drops). Full vitest green.

## Gate

vitest; tsc clean; dist rebuilt (LF). Frontend-only.
