# M391 — Artifact retrieval `agt artifact get <ref>` (SPEC-04 §3.6, final slice)

## Context
M389 added the `kernel/artifact` CAS store; M390 wired the agent loop to offload
large tool outputs (journal `tool.result` carries a `raw_ref`). The missing leg
was retrieval — there was no way to fetch the full bytes back by ref. This
milestone closes the SPEC-04 §3.6 round-trip.

## What
- **`kernel/controlplane`** — new `CmdArtifactGet` command + `handleArtifactGet`:
  takes a `ref`, fetches from `k.Artifacts().Get(ref)` (which re-verifies the
  bytes against the ref), returns `{ref, size, data(base64)}`. The store
  sentinels map to clear messages: `ErrBadRef` → "malformed ref", `ErrNotFound` →
  "artifact not found", `ErrCorrupt` → "artifact CORRUPT".
- **`cmd/agt/artifact.go`** — `agt artifact get <ref> [--out <file>]`: dials the
  daemon, decodes the base64 payload, writes the raw bytes to stdout or a file.

## Verification
- **`kernel/controlplane/artifact_test.go`** (integration, `startPair`):
  `TestArtifactGet_RoundTrip` — Put a 17 KB blob in the kernel store, fetch it
  back over the control plane, assert ref/size/bytes match; `TestArtifactGet_Errors`
  — missing ref, malformed ref (`../escape`), and a well-formed-but-absent ref all
  return clear errors (no panic, no traversal).
- **Negative control:** corrupting the handler's returned data (`base64(...WRONG)`)
  → the round-trip test FAILs (`fetched 5 vs 17000 bytes`); restored
  byte-identical.
- **Live demo** (daemon, `AGEZT_ARTIFACT_THRESHOLD=10`, `AGEZT_DEMO_LOOP`): a run
  offloaded a 109-byte shell output (ref `b7659129…`); `agt artifact get <ref>`
  to stdout printed the exact output, and `--out recovered.bin` wrote 109 bytes
  that `cmp` confirmed identical to the on-disk blob. Error paths: a malformed
  ref → "malformed ref (want a 64-hex content address)"; an absent ref →
  "artifact not found: aaaa…".
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2191** passing (was 2189; +2). CHANGELOG (Added, user-visible).

## Scope notes
- **SPEC-04 §3.6 is now functionally complete end-to-end:** store (M389) → loop
  offload + journal `raw_ref` (M390) → retrieval (M391). The model still gets the
  full output; the journal stays small; the bytes are recoverable + dedup +
  integrity-checked.
- Optional polish (recorded, not required): a web-UI link in the run-detail
  tool-call card that fetches an offloaded output via the ref (today the preview
  + raw_ref are shown; `agt artifact get` recovers the full bytes).
