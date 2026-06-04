# M313 — Planner uses JSON mode (the first consumer; arc goes live)

## Why
M311/M312 built the provider request layer for JSON mode but nothing set
`CompletionRequest.JSONMode` yet. SPEC-10 §2 names **plan generation** as the
canonical structured-output case ("reliability over free-form parsing"), and the
planner already builds its own `CompletionRequest` and parses the response as
JSON — so it's the cleanest, most spec-aligned first consumer, and a single
localized change (no run→provider interface plumbing).

## What
- **`kernel/planner/planner.go`**: the plan-generation request now sets
  `JSONMode: true`. Safe to set unconditionally:
  - Providers with a native JSON mode (OpenAI & compatibles, Gemini, Ollama)
    constrain decoding to valid JSON → fewer parse failures.
  - Providers without one (Anthropic, Bedrock-Anthropic) ignore the flag (the
    M311/M312 design), and the prompt's explicit JSON instruction still applies —
    no regression.
  - OpenAI's `json_object` precondition ("the word JSON must appear in the
    prompt") is already satisfied by the planner's ```​json-fenced instruction.

## Verification
- **`kernel/planner/planner_test.go`**: `TestGenerate_RequestsJSONMode` — a
  capturing provider records the request and asserts `JSONMode == true`.
- The arc is now provable end-to-end offline: the planner sets the flag (this
  test) → the providers encode their native JSON field when it's set
  (M311/M312 httptest tests). Same type-system proof the prompt-cache request arc
  used.
- Full suite **1976** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- The JSON-mode capability is now live: spec'd (SPEC-10) → provider request layer
  (M311 OpenAI-shaped + Ollama, M312 Gemini) → first consumer (M313 planner).
- **Remaining (optional enhancements):** OpenAI-API pass-through (set `JSONMode`
  when a client sends `response_format` — needs a `RunModel` interface change
  across openaiapi/restapi); a catalog `json_mode` capability flag so the kernel
  can record a degradation event when a non-supporting provider serves a
  JSON-mode request (SPEC-04/15) — today such a provider silently falls back to
  prompt-instructed JSON, which is correct but unrecorded.
