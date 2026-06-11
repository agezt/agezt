# Phase M823 — Files (file manager) view

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "…file manager
mantığı getirelim" — browse/preview/download/delete the stored artifacts.

## What shipped

`frontend/src/views/Files.tsx` — a file-manager view over the M822 artifact
index, registered in the Converse nav group (next to Inbox, `FolderOpen` icon):

- **Gallery + list:** image entries render as a thumbnail grid (`<img>` via the
  binary `/api/artifact/raw` route); everything else as a list (name, kind, size,
  time). Filter chips: All / Images / Files.
- **Preview modal:** click an image → larger view + metadata (source, sender,
  mime, size, time, vision caption) + Download + Delete.
- **Download:** `<a download>` to `/api/artifact/raw?…&download=1`.
- **Delete:** `useUI().confirm` → `POST /api/artifact/delete` → reload (blob GC'd
  server-side when unreferenced).
- Standard conventions: `usePanel` read, Card/Badge/SkeletonGrid/EmptyState,
  `useUI` toast/confirm, `withToken` URLs.

## Tests

- vitest `Files.test.tsx`: `isImage`/`rawURL` helpers; gallery `<img>` + file row
  render from a mocked list; delete confirms then calls `/api/artifact/delete`
  with the id; empty state. Full vitest 510 green; tsc clean.
- Live: daemon serves `/api/artifacts` (`{count:0}`) and the embedded bundle
  contains the Files view; raw route 400s without a ref.

## Scope note / backlog

This delivers the owner's core want — inbound channel images are saved (M822) and
now **shown + downloadable + deletable** in Files. Deferred to a follow-up:
indexing tool-output artifacts (so the file manager lists run outputs too — needs
threading the index through the agent loop's offload) and inline image thumbnails
in the Inbox conversation view.

## Gate

vitest 510; tsc clean; `kernel/webui/dist` rebuilt (LF). Frontend-only — no Go
change.
