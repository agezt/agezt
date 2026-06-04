# M315 — JSON mode on the OpenAI Responses API

## Why
M314 wired structured output through agezt's OpenAI **Chat Completions** API but
passed `false` on the **Responses** API path. This completes the symmetry: both
OpenAI-compatible surfaces now honour a client's structured-output request.

## What
- **`kernel/openaiapi/responses.go`**: `responsesRequest` gained `text.format`
  (the Responses API's native structured-output field) and a top-level
  `response_format` (some SDKs send it there). A `wantsJSON()` helper treats
  either `json_object` / `json_schema` as JSON mode; it flows to `RunModel`
  (non-streaming and streaming — `streamResponses` gained a `jsonMode` param like
  `streamChat`). Reuses M314's `chatRespFormat.wantsJSON()`.

## Verification
- **`kernel/openaiapi/responses_test.go`**: `TestResponses_JSONMode` — drives the
  real handler; `text.format.json_object` and top-level
  `response_format.json_schema` both set JSON mode on the run, absence leaves it
  off.
- Full suite **1978** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Additive; no behaviour change without a format field. Reuses the M314 plumbing
  end-to-end (`WithJSONMode` → `LoopConfig.JSONMode` → `CompletionRequest.JSONMode`
  → provider encoders).
- The JSON-mode capability is now complete across every surface: internal
  (planner, M313), OpenAI Chat Completions (M314), OpenAI Responses (M315) — over
  every provider with a native JSON mode (M311/M312). Only remaining: a catalog
  `json_mode` capability flag + degradation event (SPEC-04/15), where a
  non-supporting provider's silent fallback would be recorded.
