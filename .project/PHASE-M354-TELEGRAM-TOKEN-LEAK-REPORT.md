# M354 — Telegram: scrub bot token from transport errors

## Why
Priority-A security fix, found by a secret-leakage audit (grep for
`Fprintf/Errorf/Publish` touching token/secret/credential, filtering out presence
checks and `%w`-wrapped descriptions). The Telegram API is unusual: it carries the
bot token in the URL **path** (`<base>/bot<token>/getUpdates`, `/sendMessage`,
`/getFile`, `/file/bot<token>/…`). Go's `http.Client.Do` returns transport errors
as `*url.Error`, whose `Error()` text embeds the full request URL — so a routine
network failure (DNS, connection refused, timeout — common for a long-polling
channel) produces an error string containing the bot token.

`getUpdates` errors are swallowed by the poll loop today, but `Send` (sendMessage)
propagates its error to the daemon's `agt send` path and the Pulse-brief delivery
tee, where it can be surfaced to the operator or recorded. Returning the raw error
is a latent token-leak that depends on every downstream caller being careful — the
wrong default for a secret.

## What
Production security fix + lock-in test.
- **`plugins/channels/telegram/telegram.go`** — new `(*Channel) scrubToken(err)`
  that replaces the bot token with `<redacted>` in an error message (no-op when the
  token isn't present). Applied at all four `c.client.Do(...)` error returns
  (`getUpdates`, `getFile`, file download, `sendMessage`). Added `errors` import.
- **`telegram_test.go`** — `TestTelegram_TransportErrorsRedactToken`: points a
  channel with a recognizable secret token at an unreachable address
  (`127.0.0.1:1`), calls `Send`, and asserts the returned error does **not** contain
  the token and **does** contain `<redacted>` (proving the token was present in the
  raw `url.Error` and is now scrubbed).

## Verification
- `go test ./plugins/channels/telegram -run TransportErrorsRedactToken -v` — passes
  (the `<redacted>` assertion confirms the leak was real and is now closed).
- `gofmt -l` clean; `go vet ./plugins/channels/telegram/` clean; `GOOS=linux go
  build ./...` exit 0. Full suite **2086** passing (was 2085; +1), `go test ./...`
  exit 0. `go.mod`/`go.sum` unchanged. CHANGELOG updated (Security).

## Scope notes
- Telegram-specific: it is the only channel that puts a secret in the URL. Slack and
  Discord authenticate with a `Bearer` header / signed payloads, and the generic
  webhook channel uses an HMAC signature header — none embed the secret in the URL,
  so their `Do` errors don't carry it.
- The daemon already redacts known secret patterns from journalled text via
  `kernel/redact` (which includes a Telegram-bot-token pattern); this fix scrubs at
  the source so the token never even reaches a path that might bypass redaction.
