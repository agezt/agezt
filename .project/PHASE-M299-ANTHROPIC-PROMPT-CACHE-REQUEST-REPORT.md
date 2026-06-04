# M299 — Request Anthropic prompt caching

## Why
A gap surfaced while finishing the cache arc: M290/M296 parse Anthropic/Bedrock
`cache_read_input_tokens` / `cache_creation_input_tokens` from *responses*, but a
grep showed **no encoder ever set `cache_control`** on the request. Anthropic only
caches when the request marks a cache breakpoint (unlike OpenAI, which caches
automatically), so Agezt never triggered Anthropic caching — the response-side
accounting always saw zero. This adds the request-side piece, making the cache
savings (surfaced by `agt cache` / the Web UI Cache panel) real on Anthropic.

## What
- **`plugins/providers/anthropic/anthropic.go`**: `anthTool` gains an optional
  `cache_control`; new `anthCacheControl{Type}` and `buildAnthTools(tools)` which
  marks the LAST tool with `cache_control: {type: ephemeral}`. Anthropic caches
  the prefix up to and including the marked block, so this caches the whole tools
  array — the large, stable part of an agent loop's request that repeats every
  iteration. `encodeRequest` uses `buildAnthTools`.
- **`plugins/providers/anthropic/streaming.go`**: `encodeStreamRequest` uses the
  same `buildAnthTools` (both request paths now request caching).

Safe to always set: Anthropic silently ignores the marker when the cached prefix
is below the minimum cacheable size (~1024 tokens), and the cache-write premium on
the first call (1.25× the tools tokens, once) is dwarfed by the ~0.1× cache-read
rate on every subsequent iteration of a run.

## Files
- `plugins/providers/anthropic/anthropic.go`,
  `plugins/providers/anthropic/streaming.go` (edited).
- `plugins/providers/anthropic/anthropic_test.go`: **new**
  `TestEncodeRequest_PromptCacheMarksLastTool` (last tool carries
  `cache_control: ephemeral`, earlier tools don't).

## Verification
- Full suite **1910**, 68 packages, `go test ./...` exit 0; `go vet` clean;
  `gofmt -l` clean on the touched files; `GOOS=linux` build clean; `go.mod` /
  `go.sum` unchanged.
- **Network-free proof**: the httptest test drives `Complete` with two tools and
  asserts the captured request body marks only the LAST tool with
  `cache_control: {type: ephemeral}`. (The actual cache hit + token savings need a
  real Anthropic backend — the mock provider does not implement caching — but the
  response-side accounting that records them already exists, M290, and the savings
  surface in `agt cache` / the Cache panel, M293.)

## Scope notes
- Direct Anthropic only this milestone. Bedrock and Vertex Claude also never
  request caching (same gap) — wiring their encoders the same way is the immediate
  follow-up; until then they're unchanged (no regression, just not-yet-caching).
- Caches the tools prefix (present on every agent call). Caching the system prompt
  too (a second breakpoint) would require changing the `system` field from a
  string to a block array — deferred; tools are the larger, always-present prefix.
- Default-on: the change is additive and net-positive for any multi-iteration run;
  no flag. A future opt-out flag could be added if an operator ever needs it.
