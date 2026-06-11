# Phase M827 — index offloaded tool outputs (file manager shows run outputs)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** the M823 backlog item —
the file manager only listed inbound images; the owner wanted non-image
artifacts (run outputs) too.

## What shipped

- `artifact.Index.IndexRef(ref, meta, createdMs)` — records a metadata Entry for
  an ALREADY-stored blob (no re-Put), filling Size from the store. For outputs the
  agent offloaded itself.
- `wireArtifactIndexer(ctx, k)` in cmd/agezt: a bus subscriber that watches
  `tool.result` events carrying a `raw_ref` (the agent offloaded a large output to
  the blob store) and `IndexRef`s it as `kind=tool-output, source=run`, name
  `<tool>-output.txt`, with the run correlation and output_bytes. Best-effort,
  on the daemon ctx. Decoupled — the agent loop is untouched.

## Tests

- `IndexRef`: records an entry for a pre-stored ref (Size from store), lists under
  tool-output, rejects an absent ref.
- **Live** (real deepseek): an agent run that printed 9000 chars via `code_exec`
  (> the 8 KiB offload threshold) → `/api/artifacts?kind=tool-output` shows
  `code_exec-output.txt` (tool-output, run, 9091 bytes). The file manager now
  lists run outputs alongside inbound images.

## Gate

`go test` artifact + cmd/agezt green; vet + staticcheck + linux clean. No new env
var; frontend untouched (Files already renders non-image entries as a list).
