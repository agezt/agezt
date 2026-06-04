# M390 — Loop offload of large tool outputs to the artifact store (SPEC-04 §3.6 / SPEC-01 §10.2)

## Context
M389 added the `kernel/artifact` content-addressed store. This milestone is the
second SPEC-04 §3.6 slice: the agent loop now USES it — a tool output larger than
a threshold is offloaded so the journal `tool.result` event carries a preview +
`raw_ref` instead of the full bytes. The model still receives the complete output
(only the journaled event is slimmed). Verified by reading agent.go: the event
payload (`output: result.Output`) and the model's tool message were both the full
string, with no size bound on the event.

## What
- **`kernel/agent/agent.go`** — `LoopConfig.Artifacts` (an `ArtifactPutter`
  interface, satisfied by `artifact.Store`) + `ArtifactThreshold`. New pure
  `offloadToolOutput(store, threshold, output)`: above the threshold it `Put`s the
  full bytes and returns a 512-byte preview + ref; otherwise (no store, small
  output, or a `Put` error) it returns the output unchanged. Never errors —
  offload is best-effort and must not fail a run. The `tool.result` publish uses
  it: on offload the event gains `raw_ref` + `output_bytes`; the model's tool
  message keeps the full output.
- **`kernel/runtime/runtime.go`** — opens the store under `<base>/artifacts`,
  holds it on the Kernel (`Artifacts()` accessor), and passes it + `Config.
  ArtifactThreshold` into the loop's `LoopConfig`.
- **`cmd/agezt/main.go`** — `AGEZT_ARTIFACT_THRESHOLD` (positive bytes; unset →
  the 8 KiB kernel default) + config inventory entry.

## Verification
- **`kernel/agent/offload_internal_test.go`** (`TestOffloadToolOutput`, 5 subtests):
  no store → inline; small output → inline + no Put; large → offloaded with a
  preview naming the ref + the store gets the full bytes once; a Put failure →
  falls back to inline; a custom threshold is honoured.
- **`kernel/agent/offload_test.go`** (black-box, real store + journal):
  `TestRun_OffloadsLargeToolOutput` — a 20 KB tool output → `tool.result` carries
  `raw_ref` + `output_bytes`==full length + a preview shorter than the original,
  and `store.Get(ref)` returns the exact original bytes;
  `TestRun_SmallToolOutputStaysInline` — a small output is journaled inline with no
  raw_ref (no regression for ordinary results).
- **Negative control:** forcing `offloaded=false` at the publish site → the
  offload integration test FAILs ("should carry a raw_ref"); restored
  byte-identical.
- **Live demo** (daemon, `AGEZT_ARTIFACT_THRESHOLD=10`, `AGEZT_DEMO_LOOP`): a run
  produced a 109-byte shell output → journal `tool.result` carries
  `raw_ref:b7659129…` + `output_bytes:109`, and the on-disk blob
  `~/.agezt/artifacts/b7/b7659129…` is exactly 109 bytes (the content address
  matches the ref).
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged (blake3 already
  a dep). Full suite **2189** passing (was 2181; +8). CHANGELOG (Added,
  user-visible).

## Scope notes
- SPEC-04 §3.6 now: store (M389) + loop offload + RawRef event + daemon wiring
  (M390). Remaining slice: a **retrieval surface** — `agt artifact get <ref>`
  (and a web-UI link from the tool-call card's offloaded output) so an operator
  fetches the full bytes. Recorded as the next step.
- Note: keep `ArtifactThreshold` ≥ the 512-byte preview for the offload to
  actually shrink the event (the default 8 KiB does); a tiny threshold (the demo's
  10) still offloads + stores but the sub-512-byte preview is the whole output.
