# M231 — Redact Perplexity & Fireworks API keys

## Why
M230 made eight OpenAI-compatible vendors first-class (they work with just an
API key, no `custom.json`). Auditing that against the redaction set (M228)
surfaced a gap: two of those vendors' key formats are **not** redacted, so a
freshly-supported key landing in a log line, journal payload, or plugin-stderr
would leak in the clear:

- **Perplexity** — `pplx-…`
- **Fireworks AI** — `fw_…`

Verified directly: `redact.New().Redact("…pplx-<token>…")` and the `fw_` form
both passed through unredacted. (Cerebras `csk-…` is already covered — the `sk-`
rule matches its `sk-…` substring, scrubbing the token and leaving only the
leading `c`. Together/DeepInfra have no distinctive prefix to pattern on.)

This completes the redaction coverage for the vendors M230 promoted, tying the
M228 (secret formats) and M230 (first-class vendors) threads together.

## What
Two patterns added to `kernel/redact/redact.go` `namedPatterns`:

- `perplexity-key`: `pplx-[A-Za-z0-9]{20,}`
- `fireworks-key`: `fw_[A-Za-z0-9]{20,}`

Both join the shared `patterns` slice every redaction call site uses (the bus,
plugin stderr, `agt redact`, …) — no wiring change. The package doc-comment list
was updated.

## Files
- `kernel/redact/redact.go` — 2 patterns + doc-comment update (edited).
- `kernel/redact/redact_m231_test.go` — 3 tests (new): both formats redact to the
  placeholder; `MatchedCategories` labels them; and a false-positive guard
  confirms sub-floor look-alikes (`fw_handler`, `pplx-` with no token body) are
  left untouched.

## Verification
- `go test ./kernel/redact/` — green (new + all pre-existing); full suite
  **1757 → 1760** (+3), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/redact/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Proof:** redaction is a pure library exercised through the same public
  `redact.New()` the daemon uses, so the tests verify exactly what every call
  site gets (same approach as M228).
