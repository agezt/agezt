# M325 — Non-streaming reasoning capture

## Why
M317 captured reasoning by publishing ephemeral `llm.reasoning` events from the
streaming callback (`CompleteStream`). But the agent loop also has a **non-
streaming** path (`Provider.Complete`, used when the provider isn't a
`StreamingProvider` or streaming is otherwise not taken). There, the response's
`ReasoningContent` was set and its size recorded as `reasoning_chars` on
`llm.response` — but no `llm.reasoning` event carried the text. So for a non-
streaming reasoning run, the chain of thought was invisible to every consumer the
M322–M324 work added (`agt pulse`, the ACP `agent_thought_chunk` relay, the
OpenAI-compatible API's `reasoning_content` / Responses `reasoning` item). This is
the scope boundary M323/M324 explicitly flagged; this closes it.

## What
- **`kernel/agent/agent.go`**: after the non-streaming `Complete` returns, if the
  response carries `ReasoningContent`, the loop publishes one ephemeral
  `llm.reasoning` event (`PublishStreaming`, payload `{iter, text}`) — the same
  shape the streaming branch emits per delta, so every existing consumer
  (`reasoningText` in openaiapi, the ACP relay, pulse) picks it up unchanged. The
  streaming branch already emits these live, so this strictly covers the non-
  streaming path; no double-emit. Ephemeral, so the durable journal stays lean
  (only `reasoning_chars` persists, as before).

## Verification
- **`kernel/agent/agent_test.go`** `TestRun_PublishesReasoningEvents_NonStreaming`:
  a bare `Provider` (no `CompleteStream`) returning reasoning whole drives the real
  `agent.Run` over a real bus; the test asserts a single ephemeral `llm.reasoning`
  event carries the full reasoning text, alongside `llm.response`.
- End-to-end by composition with real-code tests: this test proves non-streaming
  reasoning → `llm.reasoning` event; the M323 openaiapi test proves
  `llm.reasoning` event → `reasoning_content`. The payload shape (`{iter, text}`)
  is identical across both, so the full non-streaming chain holds. (The offline
  mock provider can't emit reasoning, so a daemon curl can't add proof here — the
  real-`agent.Run` test is the end-to-end proof for this loop-level change.)
- Full suite **2011** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Off the hot path for the common case: real reasoning-capable providers (OpenAI,
  Anthropic, Google, Vertex) implement `StreamingProvider`, so they already took
  the streaming branch. This covers the case where streaming isn't used — a
  non-streaming-only provider, or a configuration that disables streaming — and
  makes reasoning capture uniform regardless.
- With M317–M321 (capture at every provider, both streaming and now non-streaming)
  and M322–M324 (surface to ACP + both OpenAI-compatible endpoints), the reasoning
  feature is complete end-to-end across every provider path and every external
  surface Agezt exposes.
