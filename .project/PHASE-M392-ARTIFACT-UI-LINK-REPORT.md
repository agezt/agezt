# M392 — Surface offloaded artifacts in the run-detail card (SPEC-04 §3.6 / SPEC-07)

## Context
M390 offloads large tool outputs (the `tool.result` event carries a `raw_ref` +
`output_bytes` + a preview); M391 added `agt artifact get <ref>` retrieval. But
the web Live Monitor's run-detail card rendered only the preview `output`, so an
operator couldn't tell an output was offloaded or how to recover it. This closes
the artifact UX — the optional web-UI slice noted in M391.

## What
- **`kernel/webui/dashboard.html`** — `tool.result` rendering now surfaces the
  offload:
  - `arcDetail` (compact): appends `⤓ <output_bytes>B artifact <ref-prefix>…` when
    `raw_ref` is present.
  - `arcFull` (expanded): the preview, then `⤓ output offloaded to the artifact
    store (<N> bytes)` + `ref: <full ref>` + `fetch: agt artifact get <ref>`.
  - XSS-safe: textContent via `el()` only (the `TestDashboard_NoUnsafeDOMSinks`
    guard still passes).

## Verification
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersOffloadedArtifact` —
  asserts the embedded HTML carries `raw_ref`, `output_bytes`, `artifact`, and the
  `agt artifact get` fetch hint.
- **Negative control:** renaming `agt artifact get` → `agt DELETED get` → the test
  FAILs on the missing marker; restored byte-identical.
- **Live demo** (daemon + web UI, `AGEZT_ARTIFACT_THRESHOLD=10`, `AGEZT_DEMO_LOOP`,
  Playwright): the run-detail `tool.result` row shows `shell ✗ … ⤓ 109B artifact
  b7659129c1ee…`; expanded it shows the error preview + `⤓ output offloaded to the
  artifact store (109 bytes) / ref: b7659129… / fetch: agt artifact get b7659129…`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2192** passing (was 2191; +1). CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-04 §3.6 is now complete with UX: store (M389) → loop offload + journal
  raw_ref (M390) → `agt artifact get` retrieval (M391) → run-detail surfacing +
  fetch hint (M392). The run-detail tool-call card surfaces are now: input/output
  (M336/M341), isolation (M379), policy (M380), context (M373), governor
  routing/capability/spend (M385), and offloaded-artifact (M392).
