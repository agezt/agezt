# M311 — Structured output / JSON mode: provider request support (slice 1)

## Why
JSON mode / structured output is an explicitly **spec'd** capability that was
deferred at M1.v and never built. SPEC-10 §2: "Structured output / JSON mode —
used wherever the system must reliably parse the LLM (plan generation, salience
scores, classifications). Reliability over free-form parsing." SPEC-04 / SPEC-15
describe providers advertising it and a parsing fallback when a provider lacks
it. Until now the request type had no way to ask for it, so every internal caller
that needs machine-parseable output relied on free-form parsing.

This is slice 1 — the **provider request encoding** — following the same
request-support-first rhythm the prompt-cache arc used (M299 added the
`cache_control` request marking before the accounting/consumers existed).

## What
- **`kernel/agent/agent.go`**: `CompletionRequest` gained a `JSONMode bool`. A
  provider with a native JSON mode honours it; one without ignores it (the caller
  keeps its robust parsing — the SPEC-10 degradation path). Default false leaves
  every request byte-for-byte unchanged.
- **`plugins/providers/openai`** (covers OpenAI **and**, via the shared encoder,
  every OpenAI-compatible vendor — Groq/DeepSeek/xAI/OpenRouter/Together/… —
  plus Azure and Mistral): `oaRequest` (and the streaming request) gained
  `response_format`; `encodeRequest` / `encodeStreamRequest` set
  `{type: "json_object"}` when `JSONMode` is set (shared `jsonObjectFormat`
  helper). `json_object` not `json_schema`, so compat vendors that only support
  the former still work.
- **`plugins/providers/ollama`**: `ollamaRequest` gained `format`; both encoders
  set `format: "json"` (Ollama's native JSON switch) when `JSONMode` is set.

## Verification
- New `json_mode_test.go` in each package: `JSONMode=true` sets the native field
  (`response_format.type == json_object` / `format == "json"`), `false` omits it,
  and the **streaming** encoder honours it too. The existing direct-encode tests
  (vision, tool-name, max-tokens) were updated for the new param.
- Full suite **1973** passing, `go test ./...` exit 0; `gofmt -l` clean on every
  changed file; `go vet` clean; `go.mod` / `go.sum` unchanged. Network-free.

## Scope notes
- Additive, default-off: no behaviour change until a caller sets `JSONMode`.
- **Remaining slices (documented next work):**
  1. Gemini family (`google` + `vertex`): `generationConfig.responseMimeType =
     "application/json"` — same shape, a clean follow-up.
  2. **Consumers**: set `JSONMode` where the system parses the model — the
     planner's plan-generation call (SPEC-10's primary example), and pass-through
     from the OpenAI-compatible API server when a client sends `response_format`.
  3. Capability advertising + fallback (SPEC-04/15): a catalog `json_mode` flag so
     the kernel only sets `JSONMode` on supporting models and records the
     degradation otherwise.
- Anthropic / Bedrock-Anthropic have no `response_format`; structured output
  there is done via tool-use/prefill — out of scope for this slice.
