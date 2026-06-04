# M322 — Reasoning reaches the editor (ACP thought chunks)

## Why
M317–M321 built the reasoning **capture** pipeline across every provider path
(direct Anthropic/Gemini, Vertex Gemini/Claude, openai-compatible DeepSeek-R1) —
reasoning streams as ephemeral `llm.reasoning` events. But at the ACP boundary
(`agt acp`, how Agezt plugs into editors like Zed) only `llm.token` answer
deltas were relayed, as `agent_message_chunk`. The captured reasoning was
silently dropped — so an editor user saw the answer appear but never the model's
thinking, even though Agezt had it. The Agent Client Protocol already defines a
distinct `agent_thought_chunk` session update for exactly this; Agezt just wasn't
emitting it. This is the consumer-side capstone of the reasoning workstream.

## What
- **`kernel/acp/acp.go`**: the `Runner.Prompt` callback gains a `ChunkKind`
  (`ChunkMessage` | `ChunkThought`). `handlePrompt` maps `ChunkThought` →
  `agent_thought_chunk` and `ChunkMessage` → `agent_message_chunk`. Reasoning
  chunks deliberately do **not** set the `streamed` flag — that flag guards the
  non-streaming answer fallback, and a run that emitted only reasoning still needs
  its answer delivered.
- **`cmd/agt/acp.go`** (`controlPlaneRunner`, the one real implementor): relays
  `llm.token` events as `ChunkMessage` and `llm.reasoning` events (M317) as
  `ChunkThought`; other kinds are dropped as before.

No protocol invention — `agent_thought_chunk` is standard ACP. No new flag:
reasoning only flows when a model produces it (thinking is itself opt-in upstream
per M318/M319/M320/M321).

## Verification
- **`kernel/acp/acp_test.go`** `TestPromptStreamsThoughtChunks`: a runner emitting
  thought + message chunks yields two `agent_thought_chunk` updates (reassembling
  to the full reasoning) and one `agent_message_chunk`, through the real `Serve`
  loop + JSON-RPC framing.
- **`cmd/agt/acp_test.go`** `TestACPRunner_RelaysReasoningAsThought`: the real
  `controlPlaneRunner` maps `llm.reasoning`→`ChunkThought`, `llm.token`→
  `ChunkMessage`, and drops an unrelated kind.
- **Live (offline)**: a standalone program drove the real `acp.Server` over a pipe
  with `session/new` + `session/prompt` (the two requests an editor sends); the
  emitted session updates were `agent_thought_chunk` for the reasoning and
  `agent_message_chunk` for the answer. Network-free.
- Full suite **2004** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Non-reasoning runs are byte-for-byte unchanged (no thought chunks emitted).
- The ACP **client** (`kernel/acp/client.go`, Agezt consuming a *peer* ACP agent)
  still surfaces only `agent_message_chunk`; relaying a peer's thought chunks back
  is a clean, separate follow-up (other direction, lower value).
- With this, the reasoning workstream (M317–M322) is end-to-end: captured at every
  provider, streamed as `llm.reasoning`, and now rendered in the editor's thinking
  UI — not just counted as `reasoning_chars`.
