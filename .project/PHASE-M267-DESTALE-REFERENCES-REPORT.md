# M267 — Correct stale references to now-shipped features

## Why
Completed work left two operator/reader-facing inaccuracies behind:
1. **`agt provider check --stream`** printed `provider family %q does not yet
   support streaming (M1.q only wires anthropic; others land in M1.q.x)` on the
   not-a-`StreamingProvider` branch. That was true when only Anthropic streamed —
   but every first-party family now implements `CompleteStream` (anthropic,
   openai, google, bedrock, vertex, cohere, ollama, and openai-compatible vendors
   via `compat`). The message is therefore both **factually wrong** and
   effectively **unreachable for real families**; it misleads an operator into
   thinking streaming is unimplemented when their provider in fact streams.
2. A **credential-vault doc comment** (`kernel/creds/encrypt.go`) still called
   `agt vault encrypt` / migration "(deferred)" — but `agt vault encrypt` has
   long shipped and `agt vault migrate` shipped in M264.

Neither is a logic bug, but both are honesty defects in shipped output/docs that
contradict what the binary actually does.

## What
- **`cmd/agt/check.go`** — extracted `streamingUnsupportedMessage(family)`: a
  pure, accurate message that names the family and points at re-running without
  `--stream`, dropping the false "only anthropic / lands in M1.q.x" claim. The
  not-ok branch in `runStreamProbe` now uses it.
- **`kernel/creds/encrypt.go`** — the format-compatibility doc comment now states
  that operators encrypt via `agt vault encrypt` and upgrade an older encrypted
  vault's key-derivation in place via `agt vault migrate` (both shipped), instead
  of calling them "(deferred)".

## Files
- `cmd/agt/check.go` — `streamingUnsupportedMessage` helper + call site (edited).
- `cmd/agt/check_stream_msg_test.go` — 1 test (new): the message names the
  family, points at dropping `--stream`, and carries none of the stale fragments
  (`M1.q`, `only wires anthropic`, `land in`, `does not yet support`).
- `kernel/creds/encrypt.go` — doc comment de-staled (no code change).

## Verification
- `go test ./cmd/agt/ -run TestStreamingUnsupportedMessage` — green; full suite
  **1857 → 1858** (+1), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised): my added lines clean. (`encrypt.go` flags a
  PRE-EXISTING struct-comment re-alignment at the envelope type, lines ~117-119 —
  untouched by this change and left alone per the standing artifact rule.)
- `go vet ./cmd/agt/ ./kernel/creds/` clean; `GOOS=linux build` clean; `go.mod` /
  `go.sum` unchanged.

## Scope notes
- Pure correctness/honesty: no behavior change beyond the corrected message text;
  the doc edit is comment-only.
- Found during a `TODO|deferred|not yet` sweep after closing the vault migrate
  arc; both items were leftovers from features that have since shipped.
