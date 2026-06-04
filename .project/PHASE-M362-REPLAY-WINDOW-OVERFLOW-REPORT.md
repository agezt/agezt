# M362 — Harden channel replay-window check against int64 overflow

## Why
Found by a duration-overflow idiom sweep (`time.Duration(n) * time.Unit` where `n`
comes from untrusted input). The Slack, Discord, and generic-webhook channels each
gate inbound messages with a freshness/replay check: reject when the message's
signed timestamp is more than `signatureWindow` (5 min) from now. All three
computed the age as:

```go
delta := abs(now - ts)              // seconds (slack/discord) or ms (webhook)
if time.Duration(delta) * time.Second > signatureWindow { reject }
```

`time.Duration(delta) * time.Second` is `delta * 1e9` nanoseconds; for a timestamp
~300 years off it exceeds `math.MaxInt64` and **wraps negative**. A negative
duration is `> signatureWindow` → **false**, so the stale check *passes* and the
message is accepted — a replay-window bypass.

This is not exploitable today: the timestamp is part of the HMAC/Ed25519-signed
payload, so forging a 300-year-off timestamp requires the secret (at which point an
attacker can do anything). But a freshness backstop should not silently overflow on
untrusted-input arithmetic — robust input handling shouldn't depend on the input
being unforgeable.

## What
Production hardening + lock-in test.
- **`plugins/channels/{slack,discord}/`** — `delta > int64(signatureWindow/time.Second)`.
- **`plugins/channels/webhook/`** — `delta > int64(signatureWindow/time.Millisecond)`.
  Integer comparison in the timestamp's own unit; no `time.Duration` conversion, so
  no overflow is possible (the difference of two unix timestamps fits int64).
- **`webhook_test.go`** — `TestInbound_OverflowTimestampRejected`: a validly-signed
  body with a fixed clock and `ts_ms` ~317 years off (delta ≈ 1e13 ms, so the old
  `delta*1e6 ns` overflows). Asserts 401 stale — with the old code the overflow
  would wrap negative and the message would be accepted.

## Verification
- `go test ./plugins/channels/{webhook,slack,discord}` — pass (incl. the new
  overflow test and the existing normal-stale tests).
- `gofmt -l` clean; `go vet ./plugins/channels/...` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2098** passing (was 2097; +1), `go test ./...` exit 0.
  `go.mod`/`go.sum` unchanged. CHANGELOG updated (Security).

## Scope notes
- Honest framing: not a live vulnerability (signed timestamp), but the correct,
  overflow-free way to compare timestamps and a defense-in-depth backstop.
- Other `time.Duration(n) * unit` sites were reviewed: the shell tool's
  `TimeoutMS` (agent-controlled; a huge value fails fast, not dangerous) and the
  vertex/metadata token `expiresIn` (provider response; a bad value just forces an
  early refresh) are low-severity and self-correcting — left as-is.
