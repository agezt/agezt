# M189 — Bounded provider HTTP response bodies (shared helper + openai)

## Why
Every provider reads its non-streaming HTTP response body with an unbounded
`io.ReadAll(httpResp.Body)`:

```go
respBytes, err := io.ReadAll(httpResp.Body)   // unbounded
```

`io.ReadAll` grows until EOF with no size limit. A provider endpoint is not always
trustworthy:
- `compat` (openai-compatible) lets an operator point a model at an **arbitrary base
  URL**, and `compat` routes those requests through the openai provider impl.
- `ollama` and custom-base-URL configs target local/arbitrary endpoints.
- Even an official endpoint can be buggy or MITM'd.

So a hostile or runaway endpoint that returns a multi-gigabyte (or never-ending) body
drives the daemon to OOM. (The SSE *streaming* path was already bounded — the scanners
use `scanner.Buffer(64KiB, 1MiB)` — so this milestone targets the non-streaming reads.)

## What
- **New shared package `plugins/providers/internal/httpread`** with
  `All(body io.Reader, max int64) ([]byte, error)`: reads at most `max` bytes via
  `io.LimitReader(body, max+1)` and returns `ErrResponseTooLarge` (with the first `max`
  bytes, so a caller can still surface a snippet) when the body is larger. `max <= 0`
  uses `DefaultMaxResponseBytes` (64 MiB — far above any legitimate LLM reply). Being
  under `plugins/providers/internal/`, it is importable by every provider package and
  nothing else.
- **Wired into the openai provider** — both read sites: the non-streaming success path
  (`openai.go`) and the streaming error-body path (`streaming.go`). openai is the impl
  that `compat` routes the untrusted custom-base-URL path through, so it's the
  highest-value first adopter and has a live `httptest` harness.

`DefaultMaxResponseBytes` is a `var` (not const) only so tests can lower it; providers
read it synchronously at call time, so a set-before-call test override is race-free.

## Tests
- `plugins/providers/internal/httpread/httpread_test.go` (white-box): under cap, exactly
  at cap, over cap → `ErrResponseTooLarge` with the truncated prefix returned, zero-max
  falls back to default, and a genuine read error is passed through.
- `plugins/providers/openai/limit_test.go` (live): with the cap lowered, an
  `httptest` server returns an 8 KiB body; `openai.Provider.Complete` returns an error
  wrapping `httpread.ErrResponseTooLarge` instead of reading unboundedly.

Existing openai/provider tests still pass (normal-size bodies unaffected).

## Verification
- `go test ./...` — 1597 passing, 0 failing (62 packages: +1 for `httpread`).
- `go vet ./plugins/providers/...` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged (`io`/`errors` are stdlib).
- Local commit only (no push); standard trailer.

## Files
- `plugins/providers/internal/httpread/httpread.go` + `_test.go` — new shared helper.
- `plugins/providers/openai/openai.go` — success read bounded (io import dropped, now
  unused).
- `plugins/providers/openai/streaming.go` — error-body read bounded.
- `plugins/providers/openai/limit_test.go` — new live test.

## Follow-ups (queued — same mechanical fix per provider)
Apply `httpread.All` to the non-streaming + streaming-error reads in the remaining
provider families, one focused milestone each (mirroring the plugin-host arc):
anthropic, bedrock, cohere, google, ollama, vertex (incl. `vertex/auth.go`'s OAuth
token read). Each has its own tests to prove no regression.
