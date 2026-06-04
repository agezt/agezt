# M301 — Cache the Anthropic system prompt too

## Why
M299/M300 cache the tools prefix (mark the last tool with `cache_control`). But
Anthropic caches the request prefix in the order **tools → system → messages**, so
a breakpoint on the last tool caches only the tools — the system prompt (also
large and stable across an agent loop) is left uncached. Adding a breakpoint on
the system block caches tools AND system, the whole stable prefix, so more of the
repeated request hits the cheap cache-read rate.

## What
- **`plugins/providers/anthropic/anthropic.go`**: `anthRequest.System` changes
  from `string` to `any`; new `anthSystemBlock` + `buildAnthSystem(system)` which
  returns nil for an empty prompt (omitted) or a one-element block array carrying
  `cache_control: {type: ephemeral}`. `encodeRequest` uses it.
- **`plugins/providers/anthropic/streaming.go`**: `encodeStreamRequest`'s
  inline request struct `System` is now `any`, set via `buildAnthSystem`.

Anthropic accepts both the string and block-array forms of `system`; the array
form is what lets it carry a cache breakpoint. Result: two breakpoints per request
— after tools (M299) and after system (M301) — caching the full stable prefix.

## Files
- `plugins/providers/anthropic/anthropic.go`,
  `plugins/providers/anthropic/streaming.go` (edited).
- `plugins/providers/anthropic/anthropic_test.go`:
  - `TestComplete_HappyPath_TextOnly` + `TestEncodeRequest_SystemFieldRespected`
    now assert the cache-marked block-array system form.
  - **new** `TestEncodeRequest_EmptySystemOmitted` (empty system → no `system`
    field at all, so a system-less request is unchanged).

## Verification
- Full suite **1913**, 68 packages, `go test ./...` exit 0; `go vet` clean;
  `gofmt -l` clean on the touched files; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Network-free proof**: the encode tests assert the system prompt is emitted as
  `[{"type":"text","text":"…","cache_control":{"type":"ephemeral"}}]`, and that an
  empty system is omitted. (The actual cache hit needs a live Anthropic backend;
  the response-side accounting records it — M290 — and it surfaces via `agt cache`
  / the Cache panel.)

## Scope notes
- Direct Anthropic only. Bedrock and Vertex Claude still send `system` as a string
  (tools-only caching from M300) — adding the system block there is the immediate
  follow-up (their request structs have `System string` too).
- The cache arc for the direct Anthropic path is now optimal: both the tools and
  the system prompt — the entire stable request prefix of an agent loop — are
  cached.
