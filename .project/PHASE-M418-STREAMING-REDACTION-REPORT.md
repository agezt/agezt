# M418 — Streaming-delta redaction + AWS secret-key detector (security)

## Context
A security review of the secret-handling surfaces (redact, creds at-rest encryption,
agent loop) found `kernel/creds/encrypt.go` and the agent loop clean (GCM nonce
uniqueness is structurally guaranteed — fresh salt+key+nonce per save, single
encryption path), and the redactor sound for its claimed scope. It surfaced one
MEDIUM exposure and one LOW coverage gap.

## Fixes

### MEDIUM — streaming deltas bypassed redaction (`kernel/bus/bus.go`)
`Bus.redactSpecLocked` ran only in `Bus.Publish`. `Bus.PublishStreaming` — used by
the agent loop for `llm.token` / `llm.reasoning` deltas — skipped it. Those events are
ephemeral (never journaled), so the **permanent** record was safe, but they are
fanned out to every matching subscriber: the outbound webhook dispatcher (whose
default subject is `>`, matching everything, with no ephemeral filter or re-redact),
the pulse stream, the OpenAI-compat relay, and the web UI. A credential the model
echoes mid-stream (e.g. re-emitting a key it read via a tool — the durable
`tool.result` that carried it *is* redacted) therefore egressed in plaintext.

Fix: `PublishStreaming` now calls `redactSpecLocked` before `NewEphemeral`, exactly
like `Publish`. Redaction is a pure deterministic transform, harmless to display.
Fixing at the source covers all subscribers at once (the right layer).

### LOW — AWS secret access key undetected (`kernel/redact/redact.go`)
The redactor caught `AKIA…` (the AWS key *id*, the non-secret half) but not the
40-char base64 *secret* access key. A bare 40-char base64 string is too ambiguous to
mask globally (it would corrupt hashes/ids), so a new templated pattern keys it to its
assignment label — `aws[_-]?secret[_-]?access[_-]?key = / : <40 chars>` (case- and
separator-insensitive, optional quotes) — masking only the secret and preserving the
label. Standalone base64 without the label is left intact.

## Verification
- **`kernel/bus/bus_test.go`** `TestRedactor_ScrubsStreamingDeltas`: a `PublishStreaming`
  carrying `sk-…` + a configured literal → the returned event and the delivered
  subscriber event are both redacted (placeholder present, secret gone), and the event
  is still ephemeral.
  - **Negative control:** removing the redact call from `PublishStreaming` → the delta
    still carries the secret → FAIL. Restored byte-identical.
- **`kernel/redact/redact_test.go`** `TestPatterns` extended (`aws-secret`,
  `aws-secret-quoted`) + new `TestAWSSecret_NotOverRedacted` (a bare 40-char base64
  with no label is left intact).
  - **Negative control:** breaking the AWS label regex → the two assignment cases
    leak → FAIL. Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2268** passing (was 2266; +2). CHANGELOG
  Security entries added.

## Review status
This closes both findings from the secret-handling review. `kernel/creds/encrypt.go`
(GCM nonce/salt/key uniqueness, PBKDF2 per RFC 6070, AEAD tamper rejection,
iteration-floor on decrypt, no padding-oracle, passphrase never logged) and the agent
loop (bounded iterations, exact-map tool dispatch, panic firewall, ctx honored,
bounded history) were found clean.
