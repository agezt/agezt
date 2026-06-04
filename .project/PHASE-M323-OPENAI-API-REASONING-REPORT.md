# M323 — Reasoning on the OpenAI-compatible API (`reasoning_content`)

## Why
M322 surfaced captured reasoning to editors over ACP. The other external surface
is Agezt's **OpenAI-compatible HTTP API** (`/v1/chat/completions`), used when a
client points at Agezt as a gateway. There, a reasoning model's chain of thought
was captured internally (M317) but never returned to the caller — the response
carried only the answer. DeepSeek-R1 and the many clients modelled on it expect
`reasoning_content` alongside the answer; this delivers it.

## What
- **`kernel/openaiapi/openaiapi.go`**:
  - `reasoningText(ev)` — extracts the text from an `llm.reasoning` event (M317),
    mirroring `tokenText`.
  - **Streaming** (`streamChat`): the live bus-subscription loop now relays
    `llm.reasoning` deltas as `delta.reasoning_content` chunks (separate from the
    answer's `delta.content`), in both the main select and the post-run drain.
    Reasoning does **not** feed the `full` accumulator that backs the usage chunk
    — it isn't the answer.
  - **Non-streaming** (`handleChat` → new `runCapturingReasoning`): subscribes to
    the run subject BEFORE launching the run (the same no-race pattern
    `streamChat` uses), runs `RunModel` in a goroutine, and accumulates
    `llm.reasoning` deltas live — the events are ephemeral, so a long chain of
    thought can exceed the bus buffer if only drained afterward. The result rides
    on `message.reasoning_content`. A failed subscription degrades to a plain run
    (reasoning is a bonus, never required). No change to the `Engine`/`RunModel`
    interface — so no fake-engine churn across the codebase.
  - When reasoning is empty the `reasoning_content` key is **omitted** entirely,
    so non-reasoning responses are byte-identical to before.

## Verification
- **`kernel/openaiapi/openaiapi_test.go`** (3 tests, real `Server.Handler()` +
  real bus): non-streaming surfaces `message.reasoning_content` while the answer
  stays clean; the key is absent when the run produced no reasoning; streaming
  emits `delta.reasoning_content` chunks distinct from the answer's `content`.
  (`fakeEngine` gained a `reasoning []string` field that publishes real
  `llm.reasoning` events to the real bus during the run.)
- **Live daemon**: built the daemon, enabled the API (`AGEZT_API_ADDR`), and
  curled `/v1/chat/completions` with the offline mock (non-reasoning) provider —
  the endpoint returned a normal completion and correctly **omitted**
  `reasoning_content` (no empty-key pollution). The reasoning-present path is
  proven by the offline httptest, which drives the identical production handler +
  bus end-to-end.
- Full suite **2007** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged. (Race detector
  unavailable — no cgo/C compiler here — but the concurrency is structurally
  identical to the production-proven `streamChat`: no shared memory between
  goroutines, the result crosses a buffered channel with happens-before, and the
  reasoning builder is touched only by the main goroutine.)

## Scope notes
- Surfaces reasoning from **streaming** runs, where `llm.reasoning` events flow
  (the kernel loop publishes them only on the streaming provider path, M317).
  Real reasoning-capable providers (OpenAI, Anthropic, Google, Vertex) implement
  `StreamingProvider`, so this is the common case. A non-streaming-only provider
  still records `reasoning_chars` on `llm.response` but wouldn't surface the text
  via the API — closing that would require threading `ReasoningContent` out of
  `RunModel`, a wider change deferred until a non-streaming reasoning provider
  actually ships.
- Scoped to Chat Completions (where `reasoning_content` is the established
  convention). The Responses API (`responses.go`) represents reasoning as
  distinct output items — a clean, separate follow-up.
- With M322 (ACP) + M323 (OpenAI API), captured reasoning now reaches **both**
  external surfaces Agezt exposes.
