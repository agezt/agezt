# M451 — Anthropic stream: tolerate a malformed structural frame

## Context
Resolving the last documented design-choice deferral: the anthropic streaming
parser aborted the whole stream on a single malformed structural SSE frame, where
the other four providers tolerate-and-continue.

## Why this is now a clear fix (not just a consistency nit)
Re-examining the parser, the strict abort is inconsistent with **its own EOF
handling**: on EOF-without-`message_stop` it deliberately `assembleResponse`s the
partial output rather than erroring, with the comment "assemble what we have so
callers see partial output rather than a hard error swallowing already-streamed
tokens." A malformed mid-stream frame should follow the same philosophy. And
skipping a frame is never worse than aborting: an abort discards *everything*
already streamed, while a skip loses only that one frame and keeps the rest. So
tolerate-and-continue is strictly better here and matches the other four parsers.

## The fix
The four STRUCTURAL frame decoders (`message_start`, `content_block_start`,
`content_block_delta`, `message_delta`) now `return nil` (skip the frame) on a
`json.Unmarshal` failure instead of returning an error that aborts the stream.
The explicit `error` SSE event is **unchanged** — it still propagates as a real
error (both the parsed and unparseable-error-frame branches), so a genuine
provider error is never silently swallowed.

## Verification
- **`plugins/providers/anthropic/streaming_malformed_test.go`**
  `TestParseStream_ToleratesMalformedFrame`: a stream with a broken
  `content_block_delta` *between* two valid text deltas ("pong" … broken … "!")
  parses with no error and yields `"pong!"` — the text on both sides of the bad
  frame is preserved.
  - **Negative control:** restore the strict `return fmt.Errorf(...)` on
    `content_block_delta` → the test FAILs ("a malformed mid-stream frame must not
    abort the stream: … invalid character 'B' …"). Restored.
- **Regression:** the existing `TestParseStream_ErrorFrame` (the `error` event
  still returns an error) and `TestParseStream_TextOnly` both still pass, and the
  M449 `FuzzParseStream` covers arbitrary malformed input.
- **Gate:** gofmt-clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged, full suite exit 0. CHANGELOG Reliability entry.

## Review status
This resolves the final documented design-choice deferral. All four provider
streaming parsers now tolerate a malformed structural frame consistently. The only
remaining deferrals are external-wire / by-design items that cannot be implemented
or verified offline (embeddings provider, cloud credential wires, Docker/CI/k8s,
env-only config, journal-0644, plaintext non-loopback) — none are offline-actionable
defects.
