# M312 — JSON mode: Gemini family (slice 2)

## Why
Completes the provider-encoding layer for structured output (M311 did the
OpenAI-shaped providers + Ollama). Gemini — both the direct Google Generative
Language API and Gemini-on-Vertex — has a native JSON mode via
`generationConfig.responseMimeType`.

## What
- **`plugins/providers/google/google.go`** + **`plugins/providers/vertex/vertex.go`**:
  `geminiGenConfig` / `vxGenConfig` gained `responseMimeType`; `encodeRequest`
  (shared by each provider's streaming + non-streaming paths) takes `jsonMode`
  and sets `responseMimeType: "application/json"` when `JSONMode` is set. The
  generationConfig is now created when **either** `maxTok > 0` **or** `jsonMode`
  is set (previously only for maxTok), and the two compose.

## Verification
- New `json_mode_test.go` in each package: `JSONMode=true` sets
  `responseMimeType=application/json`, `false` omits it, and it composes with
  `maxOutputTokens`.
- Full suite **1975** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `go.mod` / `go.sum` unchanged. Network-free.

## Scope notes
- The provider-request layer for JSON mode is now complete across every provider
  with a native JSON mode: OpenAI + all OpenAI-compatibles + Azure + Mistral
  (M311), Ollama (M311), Google + Vertex-Gemini (M312). Anthropic/Bedrock-Anthropic
  have no `response_format` (structured output there is tool-use/prefill) — out of
  scope.
- **Remaining (the consumer slice):** set `JSONMode` where the system parses the
  model — the planner's plan-generation call (SPEC-10's primary example) and
  OpenAI-API pass-through — plus a catalog `json_mode` capability flag so the
  kernel only sets it on supporting models and records the degradation otherwise.
