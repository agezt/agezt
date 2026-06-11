# Phase M828 — Inbox inline image thumbnails

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** M823 backlog — show
inbound images inline in the Inbox conversation view (not just the Files view).

## What

Inbound channel images are persisted as artifacts keyed by the run correlation
(M822); the inbox folds channel events into threads keyed by the SAME correlation
(the channel handler runs under it — verified `c.handler(ctx, msg, corr)` uses the
emitInbound corr). So they match exactly.

`Inbox.tsx` now also fetches `/api/artifacts?kind=image`, buckets entries by
`corr`, and renders a thumbnail strip under each thread whose correlation has
images. Each thumbnail is an `<img>` (via the binary `/api/artifact/raw` route)
linking to the full image in a new tab; the vision caption is the title. The
fetch is best-effort — a failure never breaks the inbox. Reuses `rawURL` +
`ArtifactEntry` from Files (M823).

## Tests

- vitest `Inbox.test.tsx`: a thread with a matching image (corr c1) renders
  exactly one `<img>` with the raw URL; an image for another thread is NOT shown.
  Full vitest 511 green; tsc clean.

## Gate

vitest 511; tsc clean; dist rebuilt (LF). Frontend-only.
