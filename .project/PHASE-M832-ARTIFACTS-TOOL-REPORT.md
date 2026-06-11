# Phase M832 — `artifacts` tool: agent lists/reads/deletes its own saved files

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "daha fazla tool
ekle bize gerekebilen şu anki toollar yetersiz gibi" — continuing the more-tools
arc; the natural companion to `fetch` (M831).

## Gap

`fetch` (M831), the tool-output offloader (M827), and inbound-image persistence
(M822) all PUT files into the artifact store / Files view. But the agent had no
way to read them back OUT: it got an `id` from `fetch` yet couldn't enumerate
"what files do I have", pull a saved file's bytes into context, or delete one —
the Files view was operator-only. So a file saved in one run was unusable in the
next.

## What shipped (`plugins/tools/artifacts/`)

A new in-process `artifacts` tool — a read/list/delete view over the existing
artifact index (no new store, no network):

- **`op=list {kind?, source?, corr?, limit?}`** → metadata of the saved files
  (`{id, name, mime, kind, source, size, caption?}`), newest-first, capped
  (default 50, `truncated` flagged).
- **`op=read {id}`** → a **text** file's contents inline (header + body, capped at
  256 KiB with a truncation note). A **binary** file instead returns its metadata
  + "download from the Files view" — raw bytes never flood the model context.
  Text-vs-binary is decided by mime (text/*, json/xml/yaml/svg, +json/+xml) or,
  for an unknown mime, a NUL-byte sniff.
- **`op=delete {id}`** → removes the entry (and GCs the blob when unreferenced,
  via the index's existing dedup-aware Delete).

Decoupled from the kernel via an `Index` interface (List/Bytes/Delete); the daemon
injects the real `*artifact.Index` post-Open with `SetIndex(...)`, mirroring the
fetch (M831) and indexer (M827) wiring. Fails soft without it.

## Wiring

- `cmd/agezt/main.go` `buildTools`: register `artifacts`
  (`artifacts(list/read/delete)` in the startup tools line), inject the index
  post-Open. Always registered.
- `kernel/edict/toolmap.go`: `case "artifacts"` → `op=delete` ⇒ `CapFileDelete`,
  else (list/read, and any garbled op) ⇒ `CapFileRead`. Artifacts are files →
  reuses the file capabilities (no new grant, no new env).

## Verification

- **Unit** (`artifacts_test.go`): list (+kind filter), text read returns content,
  binary read reports metadata (no byte dump), delete propagates + missing-id is a
  soft error, unknown op / read-without-id / missing-index all rejected.
- **Live** (isolated `AGEZT_HOME`, real deepseek agent): one prompt drove all
  three ops — `fetch https://go.dev/robots.txt` → `artifacts list` (showed the one
  `text/plain`, source=fetch entry) → `artifacts read <id>` returned the exact
  contents (`User-agent: *` / `Allow: /`). Closed the save→list→read loop.

## Gate

artifacts + fetch + edict tests green; vet + staticcheck + linux cross-build
clean; gofmt swept. go.mod unchanged (stdlib + existing agent/artifact only). No
new env var → no `configEnvVars` change. No network surface; default-allow posture
preserved (on by default, restriction only via the existing file-capability
opt-outs).
