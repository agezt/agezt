# M228 — Redaction coverage for agezt's own integration secrets

## Why
Secret redaction (M15, hardened M170) scrubs high-confidence secret *formats*
from anything the daemon logs, journals, or returns — a defence-in-depth net for
secrets the daemon was never explicitly told about. The pattern set covered
OpenAI/Anthropic `sk-…`, AWS `AKIA…`, GitHub, the Slack `xox…` family, Google
`AIza…`, JWTs, bearer tokens, and PEM keys.

But it missed several formats that **agezt itself handles** — so a key for one of
agezt's own integrations could appear in a log line, a tool result, or a journal
payload and go out unredacted:

- **Telegram bot tokens** (`AGEZT_TELEGRAM_TOKEN`) — the Telegram channel.
- **Slack app-level tokens** (`xapp-…`) — the Slack channel uses these alongside
  the `xox…` bot/user tokens that *were* covered.
- **Groq** (`gsk_…`) and **xAI** (`xai-…`) API keys — both first-class compat
  providers. The broad `sk-…` rule does **not** match `gsk_…` (no `sk-`) or
  `xai-…`, so these leaked.

A secret agezt is configured with leaking through its own redaction net is the
worst kind of gap — exactly the case redaction exists to prevent.

## What
Four new high-confidence patterns in `kernel/redact/redact.go` `namedPatterns`
(so they join the same `patterns` slice every existing redaction call site uses —
no wiring change):

- `telegram-bot-token`: `[0-9]{8,10}:[A-Za-z0-9_-]{35}`
- `slack-app-token`: `xapp-[0-9]+-[A-Za-z0-9-]{10,}`
- `groq-key`: `gsk_[A-Za-z0-9]{20,}`
- `xai-key`: `xai-[A-Za-z0-9]{20,}`

Each is a token shape implausible as ordinary prose (the redaction bar). The
package doc comment's covered-formats list was updated to match.

## Files
- `kernel/redact/redact.go` — 4 patterns + doc-comment update (edited).
- `kernel/redact/redact_m228_test.go` — 3 tests (new): the four formats are
  redacted to the placeholder; `MatchedCategories` returns the right labels; and
  a false-positive guard confirms ordinary text (`123456789:ok`, a bare `gsk`,
  the word `xai`, `12345678:30`, `xapp-thing`) is left untouched.

## Verification
- `go test ./kernel/redact/` — green (new + all pre-existing redaction tests,
  including the existing no-false-positive test, still pass — the new rules don't
  over-match); full suite **1748 → 1751** (+3), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/redact/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Proof:** redaction is a pure library; the tests exercise it through the same
  public `redact.New()` constructor the daemon instantiates, so what they verify
  is exactly what every call site (journal writes, tool-output logging, `agt why`,
  channel egress) gets. No new integration to stand up — the patterns slot into
  the already-wired mechanism.

## Scope notes
- Formats without a distinctive prefix (e.g. a raw AWS *secret* access key, or a
  Together.ai hex key) remain out of scope — there's no high-confidence shape to
  match without risking false positives on ordinary hex/base64. The literal-secret
  path (the daemon redacting keys it loaded from the creds store) already covers
  those when the daemon knows them.
