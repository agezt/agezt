# Phase M842 — Files view: in-app preview for all artifact types

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "files kısmında
önizleme vs de lazım, sadece download olmaz."

## What shipped

The Files preview modal (M823) only previewed images; everything else said "No
inline preview — use Download." Now it renders inline:

- **Images / SVG** — as a picture (unchanged; SVG is `image/*`).
- **PDF** — embedded in an `<iframe>`.
- **Markdown** — rendered with the `<Markdown>` component.
- **JSON** — pretty-printed (`prettyJSON`, falls back to raw on parse error).
- **Code / text / CSV / logs / yaml / …** — fetched and shown in a monospace
  `<pre>` (classified by mime or extension).
- **True binaries** — still fall back to a Download prompt.

`frontend/src/views/Files.tsx`: new `isPdf` + `textKind` classifiers (exported,
unit-tested) + a `PreviewBody` component that fetches text-like bytes from the
existing `/api/artifact/raw` route (token in the URL) and renders by kind, with a
2 MiB inline cap, a loading state, and graceful error fallback. No backend
change.

## Verification

- **Unit** (`Files.test.tsx`): `isPdf` (mime/extension), `textKind`
  (markdown/json/code/text + binary/image → ""). Existing Files tests still green.
- Frontend `tsc` + **517 vitest** green; Go build + webui tests green; dist
  rebuilt (LF). The raw route that serves the bytes was already verified live in
  M823/M831.

## Gate

frontend build + vitest green; Go build + webui tests green; dist committed LF, in
sync; go.mod unchanged.
