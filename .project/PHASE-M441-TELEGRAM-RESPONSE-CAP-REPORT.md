# M441 — Telegram Bot API responses: size-cap the JSON decode

## Context
Resolving the deferred LOW: *"telegram getUpdates uncapped."* Every other HTTP
response in the tree is read through a size cap (providers via `httpread.All`,
inbound channels via `io.LimitReader`, the Telegram photo download via
`tgPhotoMaxRaw`) — but the Telegram Bot API *control* responses were decoded
straight off the socket.

## The gap
`getUpdates` (`json.NewDecoder(resp.Body).Decode(&out)`) and `getFile` (same)
decoded the response body with no size bound. A buggy, compromised, or MITM'd
Bot API endpoint (or a misconfigured proxy in front of `api.telegram.org`) could
stream an unbounded body and OOM the daemon's long-poll loop. Lower severity than
the untrusted-internet inbound surface — the endpoint is operator-configured — but
it is the one HTTP response class in the tree without the size cap the rest
uniformly applies, and the long-poll loop runs continuously.

## The fix
Added `tgAPIMaxResponseBytes = 8 << 20` (8 MiB — far above any legitimate
`getUpdates` batch) and wrapped both decodes in
`json.NewDecoder(io.LimitReader(resp.Body, tgAPIMaxResponseBytes))`. An over-cap
body is truncated, so `Decode` returns an error (handled as a normal
getUpdates/getFile failure and retried) instead of buffering without bound.
`sendMessage` only checks status (no body decode), so it needs no cap.

## Verification
- **`plugins/channels/telegram/telegram_test.go`**
  `TestGetUpdates_CapsOversizedResponse`: an httptest server returns a valid-prefix
  JSON whose total size exceeds the cap (an 8 MiB text field); `getUpdates` must
  return an error (the capped reader truncates it) rather than accept it.
  - **Negative control:** remove the `io.LimitReader` wrap → the oversized body is
    accepted and `getUpdates` succeeds → the test FAILs. Restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2315** passing (was 2314;
  +1), `go test ./...` exit 0. CHANGELOG Security entry.

## Review status
Every HTTP response decode in the tree now carries a size cap. Remaining
documented-deferred items are either deliberate design (anthropic strict stream
abort — a defensible strict-vs-lenient call returning a clean error) or low-value
niceties (slack/discord async `context.Background` detach) tracked in next.md.
