# M314 — OpenAI-API JSON-mode pass-through (response_format → run)

## Why
agezt advertises OpenAI compatibility, and `response_format` is a documented
Chat Completions feature. Until now agezt's OpenAI-compatible API silently
dropped it: a client asking for `{"type":"json_object"}` got free-form output.
This wires `response_format` through to the provider's JSON mode (M311/M312), so
an external client gets structured JSON from any provider that supports it — the
second consumer of the JSON-mode capability (after the planner, M313), and the
one that makes it usable from outside the daemon.

## What
The flag follows the exact path image attachments use (a per-run value carried
on the run context, read when the loop builds `CompletionRequest`):
- **`kernel/runtime/runtime.go`**: new `WithJSONMode` / `jsonModeFromCtx`
  (ctx-value, like `WithImages`); the loop config is built with
  `JSONMode: jsonModeFromCtx(runCtx)`.
- **`kernel/agent/agent.go`**: `LoopConfig.JSONMode` → `CompletionRequest.JSONMode`
  on every provider call of the run.
- **`kernel/openaiapi/openaiapi.go`**: `chatRequest` gained `response_format`;
  `json_object` / `json_schema` set the run to JSON mode (chat — streaming and
  non-streaming).
- **`Engine.RunModel`** (openaiapi + restapi interfaces, `kernelAPIEngine` impl)
  gained a `jsonMode` param, mirroring `images` — kept the API packages decoupled
  from `runtime` (the impl calls `WithJSONMode`). The native REST API and the
  Responses API pass `false` (the Responses API's `text.format` nesting is a
  documented follow-up).

## Verification
- **`kernel/openaiapi/openaiapi_test.go`**: `TestChat_ResponseFormatJSONMode` —
  drives the real `Handler().ServeHTTP` path; `response_format: json_object`
  (and `json_schema`) set JSON mode on the run, its absence leaves it off.
- End-to-end offline: API parses response_format → run carries JSONMode (this
  test) → loop sets CompletionRequest.JSONMode → providers encode their native
  field (M311/M312 httptest). Every link tested.
- Full suite **1977** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Additive: a request without `response_format` is byte-for-byte unchanged; a
  provider without a native JSON mode ignores the flag.
- The JSON-mode capability is now complete on its two primary surfaces: internal
  (planner, M313) and external (OpenAI Chat Completions API, M314).
- **Remaining (optional):** the OpenAI **Responses** API (`text.format`) and the
  catalog `json_mode` capability flag + degradation event (SPEC-04/15) — today a
  non-supporting provider silently falls back to prompt-instructed JSON, which is
  correct but unrecorded.
