# M335 — Email channel (outbound, SMTP)

## Why
Continuing the SPEC-04 channel sweep under the v1.0-conformance goal. After the
generic webhook channel (M334), email is the next offline-verifiable channel: it
delivers Pulse briefs and `agt send` to operator inboxes over SMTP. Outbound email
is genuinely useful (get notified by mail when something matters) and fully
testable offline; inbound email (IMAP/MX) needs a live mailbox and is out of scope.

## What
- **`plugins/channels/email/email.go`** (new package): an outbound `channel.Channel`
  over stdlib `net/smtp` (no new dependency).
  - `Send` renders an RFC 5322 message (From/To/Subject/Date/MIME, CRLF-normalized
    text/plain UTF-8 body) and delivers via SMTP. The "channel_id" is the recipient
    address; a fail-closed Allowlist restricts which addresses may be mailed (a
    misconfigured brief can't spray arbitrary recipients). SMTP AUTH PLAIN is used
    when a username+password are set. The subject is derived from the priority
    (`Agezt [urgent]: …`) and the first body line, length-bounded.
  - The SMTP transport is an injectable `SendFunc` (default `smtp.SendMail`), so
    message construction is unit-testable without a live server.
  - Outbound-only: `Start` blocks until ctx is cancelled (uniform lifecycle); no
    inbound surface, so no injection risk. Credentials never logged. Journals
    `channel.outbound`.
- **`cmd/agezt/main.go`**: `buildEmail` wires it from env (`AGEZT_EMAIL_SMTP_ADDR`
  / `_FROM` / `_USERNAME` / `_PASSWORD` / `_RECIPIENTS`), starts it, registers it
  for `agt send` (`liveChannels["email"]`) and the Pulse brief tee, and lists it in
  `agt status`.
- **`kernel/controlplane/config.go`**: registered the 5 env vars (the config-
  inventory guard test enforces this).

## Verification
- **`plugins/channels/email/email_test.go`** (5 tests): message construction
  (envelope, headers, derived subject, CRLF body) via the injected sender;
  allowlist gating (non-allowlisted → error, nothing sent); empty-recipient and
  no-SMTP-addr guards; subject derivation (priority prefix, first line,
  truncation); and a **real `net/smtp.SendMail`** delivery against a minimal
  in-process SMTP server (proving the actual transport, not just the seam).
- **Live daemon**: started with the channel configured → banner reports
  "outbound via smtp.example.com:587, 1 recipient(s)" (wiring confirmed).
- Full suite **2044** passing, `go test ./...` exit 0 (two clean runs); `gofmt -l`
  clean; `go vet` clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- Outbound-only by design; inbound email is a separate, live-mailbox-dependent
  effort (IMAP poller or inbound MX) — not offline-verifiable, so deferred.
- Channels now: Telegram, Slack, Discord, generic webhook (M334), email (M335).
  Remaining SPEC-04 channels (SMS/WhatsApp/Signal/Matrix/Teams/HomeAssistant) all
  need live external services to verify.
