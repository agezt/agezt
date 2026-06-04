# M324 — Reasoning on the OpenAI Responses API

## Why
M323 surfaced reasoning on Chat Completions (`/v1/chat/completions`) as
`reasoning_content`. Agezt also exposes OpenAI's newer **Responses API**
(`/v1/responses`), which it left as an explicit follow-up. The Responses API
represents reasoning differently — as a `reasoning` output item with a
`summary_text`, and as `response.reasoning_summary_text.*` streaming events. This
brings reasoning to that surface too, so it's uniform across both OpenAI-
compatible endpoints.

## What
- **`kernel/openaiapi/responses.go`**:
  - `responseObject` gains a `reasoning` parameter. When non-empty it prepends a
    `reasoning` output item — `{type:"reasoning", summary:[{type:"summary_text",
    text:…}]}` — before the assistant `message` item, the position and shape the
    Responses API uses. Empty reasoning leaves the output array exactly as before
    (one message item).
  - **Non-streaming** (`handleResponses`): reuses M323's `runCapturingReasoning`
    (subscribe-before-run + live accumulation) to collect the run's reasoning and
    passes it to `responseObject`.
  - **Streaming** (`streamResponses`): a reasoning model's chain of thought
    streams as `response.reasoning_summary_text.delta` events (and a closing
    `.done`), distinct from the answer's `output_text` deltas, and lands in the
    final `response.completed` object as the same `reasoning` output item.

Reuses the existing simplified Responses SSE style (the handler already omits the
full `output_item.added`/`content_part` ceremony and even mints a fresh `msg_` id
for the final object distinct from the streamed `item_id`); the reasoning path
matches that simplicity — its own `rs_` item id, `reasoning_summary_text` deltas,
and the item reflected in the completed object.

## Verification
- **`kernel/openaiapi/responses_test.go`** (3 tests, real `Server.Handler()` +
  real bus via `fakeEngine`'s `reasoning` events): non-streaming yields a
  `reasoning` item (summary_text) prepended before the message, answer unpolluted;
  a non-reasoning run carries exactly the message item (no reasoning item — output
  byte-identical); streaming emits `reasoning_summary_text.delta`/`.done` and the
  reasoning item in `response.completed`, with the answer still on `output_text`.
- All pre-existing Responses tests unchanged and green (non-reasoning output shape
  preserved).
- Full suite **2010** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged. (The offline
  httptest drives the identical production handler + bus end-to-end — the live
  proof for this surface; M323's live-daemon smoke already covered the shared API
  server's health.)

## Scope notes
- Same streaming-only capture boundary as M323: reasoning flows for streaming runs
  (where `llm.reasoning` events are published, M317). Real reasoning-capable
  providers stream.
- The simplified SSE keeps both items nominally at `output_index 0` in the deltas
  (consistent with the existing handler, which never emitted `output_item.added`);
  clients key on event type + `item_id`. A fully ceremonious Responses event
  stream (output_item.added/done, reasoning_summary_part.added/done, correct
  output indices) would be its own milestone and is not required for clients to
  read the reasoning.
- With M322 (ACP), M323 (Chat Completions), and M324 (Responses), Agezt's captured
  reasoning now reaches every external surface it exposes.
