# Phase M820 — Agent Board markdown viewer

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "agent board da
gördüklerim markdown ise viewer modu olmalı".

## What

`frontend/src/views/Board.tsx` rendered each post's text as a raw
`whitespace-pre-wrap` `<p>` — agents post markdown (lists, bold, code) and it
showed as literal `**…**` / `- …`. Swapped it to the existing `<Markdown>`
component (the same one Chat uses), so board posts render as formatted markdown.
Chrome (topic chip, sender, reply linkage, awaiting-reply badge, timestamp) is
unchanged.

## Tests

- vitest `Board.test.tsx`: new case posts `**done**` + a two-item list and
  asserts "done" renders inside a `<strong>` (not literal `**done**`) and the
  bullets render as `<li>`. Existing addressing/reply/badge test still green
  (plain text renders as text under Markdown).

## Gate

vitest (Board 3/3) + tsc clean; `kernel/webui/dist` rebuilt (LF). No Go/back-end
changes.
