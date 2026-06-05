# M431 — Inbound channel HTTP servers: slow-loris timeouts

## Context
Review of the inbound channel handlers (Telegram, Slack, Discord, webhook), which
receive UNTRUSTED messages from the public internet. The high-priority concerns were
found **clean**: malformed-JSON panics are contained (per-channel — net/http per-request
recovery for the synchronous decoders, `channel.Guard` around the async runs), inbound
bodies are size-capped (`io.LimitReader` 1 MiB on Slack/Discord/webhook, HMAC/Ed25519
verified over the capped bytes so truncation fails auth), attachment fetches are capped,
and echo/reply loops are closed. The streaming-provider review (parallel) also found all
four parsers clean (bounded scanner, ErrTooLong surfaced, no panic on malformed events).
One MED reliability finding remained.

## The bug
`plugins/channels/{slack,discord,webhook}.go`: each inbound `http.Server` set only
`ReadHeaderTimeout: 10s`. That bounds the header phase, but a client that completes the
headers and then drips the POST body one byte at a time holds the handler goroutine and
the connection open indefinitely — the `io.LimitReader` caps total bytes, not time. Many
such connections exhaust goroutines/FDs (a slow-loris DoS), and the body must be fully
read before signature verification, so it is reachable pre-auth. Same class as the main
HTTP surfaces hardened in M419.

## The fix
Each channel's server construction is extracted into a `newHTTPServer()` method that
also sets `ReadTimeout` (30s — bounds the entire request read incl. body) and
`IdleTimeout` (60s). `WriteTimeout` is deliberately left unset: the webhook channel
writes its reply after a (possibly slow) synchronous agent run, and `ReadTimeout` does
not cover the post-read handler work, so the slow-body drip is bounded without cutting
off a legitimate slow reply. These channels don't stream responses, so `ReadTimeout` is
safe (unlike the SSE-bearing main servers in M419, which use only ReadHeaderTimeout).

## Verification
- **`plugins/channels/{slack,discord,webhook}/slowloris_test.go`**
  `TestNewHTTPServer_SlowLorisTimeouts`: the constructed server has non-zero
  `ReadHeaderTimeout`, `ReadTimeout`, and `IdleTimeout`.
  - **Negative controls:** removing `ReadTimeout` from each `newHTTPServer` → the
    respective test FAILs. All three restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2290** passing (was 2287; +3). CHANGELOG
  Reliability entry.

## Deferred review findings (documented, not fixed)
- MED: channel HTTP *clients* (reply send + attachment fetch) bypass `netguard` — a
  defense-in-depth gap, but not directly exploitable: the fetched URLs come from
  signature-verified payloads (Slack/Discord CDN), so only Slack/Discord can deliver one.
- LOW: Slack/Discord async runs use `context.Background()` (detached from daemon-shutdown
  cancellation) — a clean-shutdown/goroutine-drain nicety, not a crash.
- LOW: Telegram `getUpdates` response decode has no size cap (operator-configured
  upstream, outside the untrusted-internet threat model).
- LOW (providers): the anthropic streaming parser aborts the whole stream on one
  malformed structural frame where the other three tolerate-and-continue — a clean error,
  not a crash; a consistency gap.

## Review status
The inbound channel parse/handler paths and all four provider streaming parsers are
reviewed and sound. This closes the channel/provider review; the deferred items above are
documented accepted gaps.
