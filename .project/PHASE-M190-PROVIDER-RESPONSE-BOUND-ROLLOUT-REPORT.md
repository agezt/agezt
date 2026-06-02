# M190 — Provider response-body bound rolled out to all remaining families

## Why
M189 introduced the shared `httpread.All` bound and applied it to the openai provider
(the `compat`/custom-base-URL untrusted-endpoint target). The same unbounded
`io.ReadAll(httpResp.Body)` existed in every other provider family, so each one could
still OOM the daemon on an oversized response. This milestone completes the rollout so
**every** provider's response read is bounded.

## What
Applied `httpread.All(body, httpread.DefaultMaxResponseBytes)` to all remaining
`io.ReadAll` response-body sites across the provider families:

- **anthropic** — `anthropic.go` (success), `streaming.go` (error body).
- **bedrock** — `bedrock.go`, `streaming.go`.
- **cohere** — `cohere.go`, `streaming.go`.
- **google** — `google.go`, `streaming.go`.
- **ollama** — `ollama.go`, `streaming.go`.
- **vertex** — `vertex.go`, `anthropic.go` (success + error), `streaming.go`, and
  `auth.go` (the OAuth JWT-bearer → access-token exchange read).

The unused `io` import was dropped from the files where the bounded read was the only
`io` use; `streaming.go` files and `vertex/anthropic.go` keep it (still used by
`parseStream(io.Reader)` / other reads). Imports were re-grouped with `gofmt`.

## Tests
This is a mechanical rollout of the helper proven live in M189, so correctness rests on:
- **No regression**: every provider's existing `*_test.go` exercises its success read
  path against an `httptest` server returning a normal-size body; all still pass.
- **Mechanism (already proven)**: the openai live test (M189) demonstrated an over-cap
  body yields `ErrResponseTooLarge` rather than an OOM, using the identical helper call.
- **Cross-dialect proof (new)**: `plugins/providers/anthropic/limit_test.go` repeats the
  live over-cap test against the anthropic provider (a different request/response
  dialect), confirming the bound fires regardless of provider — `Complete` returns an
  error wrapping `httpread.ErrResponseTooLarge`.

## Verification
- `go test ./...` — 1598 passing, 0 failing.
- `go vet ./plugins/providers/...` clean.
- `gofmt -l` clean on my added lines (CRLF-normalized). Note: `vertex/auth.go` carries a
  PRE-EXISTING gofmt artifact (struct comment/field alignment) present in `HEAD` and
  untouched by this change — left per the pre-existing-artifact policy; my edits to it
  (import swap + bounded read) are gofmt-clean.
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `plugins/providers/{anthropic,bedrock,cohere,google,ollama,vertex}/*.go` — bounded
  reads + import adjustments (non-test files).
- `plugins/providers/anthropic/limit_test.go` — new cross-dialect live test.

## Status
The provider-response-OOM class (M189 + M190) is now fully closed across all 7 provider
families. Combined with the earlier untrusted-input bounds — plugin host (M177),
mcpbridge (M185), control plane (M188) — every newline/stream/body read from an
untrusted or operator-configurable peer in the daemon is now size-bounded.
