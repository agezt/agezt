# Phase M822 — artifact index + persist inbound channel images

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "mesajlaşma ile
gelen tüm resimleri saklayıp gösterebilsek… resim dışındaki artifactları da
saklayıp file manager mantığı getirelim." This is the backend foundation; M823
adds the file-manager UI + Inbox thumbnails.

## Why

Inbound channel images were ephemeral data-URLs, used for one run and discarded.
The blob artifact store had no metadata and no list/delete — only point-lookup by
content ref. To save + browse + delete media, we need a queryable index.

## What shipped (backend)

- **Artifact index** (`kernel/artifact/index.go`): a metadata sidecar over the
  pure blob `Store`. `Entry{ID,Ref,Name,Mime,Kind,Source,Sender,Corr,Size,
  CreatedMs,Caption}` persisted one JSON file each under `artifacts/index/`; blobs
  stay content-addressed/deduped. Methods: `OpenIndex`, `PutEntry`, `List(Filter)`
  (newest-first, kind/source/corr filters), `Get`, `Bytes`, `Count`, `Delete`
  (GCs the blob only when no other entry references the ref). Wired into the
  kernel (`k.ArtifactIndex()`).
- **Persist inbound images** (`cmd/agezt/main.go`): `makeChannelHandler` now
  decodes each `msg.Images` data-URL and `PutEntry`s it as `kind=image,
  source=<channel>, sender, corr`, with the M821 vision caption attached. Even
  when no vision model is keyed (run can't proceed), the image is still saved.
  Helpers `decodeDataURL` + `extForMime`.
- **Control plane**: `CmdArtifactList` (filtered, metadata only) + `CmdArtifactDelete`
  (by id) added; `CmdArtifactGet` unchanged.
- **Web UI routes**: `/api/artifacts` (list), `/api/artifact/delete` (POST), and a
  binary `/api/artifact/raw?ref=&mime=&download=&name=` that proxies CmdArtifactGet,
  decodes, and serves bytes with a **sanitized** Content-Type (image/doc allowlist
  → else octet-stream; nosniff is set globally) for `<img src>`/downloads.

## Tests

- index: put/list(filter)/get/bytes; delete GCs an orphan blob but keeps a shared
  one; entries persist across reopen.
- raw route: token-gated (401), serves bytes with the right Content-Type, hostile
  mime → octet-stream, missing ref → 400.
- decodeDataURL / extForMime parsing table.
- Live: daemon boots with the index; `/api/artifacts` → `{count:0,entries:[]}`.

## Gate

`go test` on artifact/runtime/controlplane/webui/cmd-agezt green; vet +
staticcheck + linux clean. Two new control-plane commands; no new env var.
Frontend untouched (no dist change) — M823 adds the Files view + Inbox thumbnails
and indexes tool outputs too.
