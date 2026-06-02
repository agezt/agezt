# M170 — Secret-redaction hardening (security review)

## Why
A security review was commissioned on `kernel/redact` — the chokepoint that scrubs
secrets before they reach the journal. The stakes are uniquely high: the journal is
append-only and BLAKE3 hash-chained, so a secret that slips through is recorded
**permanently** and can't be scrubbed without breaking the chain. The review
confirmed the redactor is concurrency-safe, mutation-safe, has no journal-write
bypass, a non-re-matching placeholder, and no ReDoS (RE2) — and found real leak
paths, three of which are fixed here.

## Root insight
Redaction scrubs the **marshaled JSON text**, not the structured values. So any
transformation `json.Marshal` applies to a secret happens *before* the scrubber
sees it — the scrubber then matches against a mangled form that no longer contains
the literal. Two of the findings stem from this.

## Fixes

### Critical — HTML-escaping defeated the literal scrubber
`json.Marshal` HTML-escapes `&`→`&`, `<`→`<`, `>`→`>` by default.
The literal scrubber searches for the *raw* configured secret, so any secret
containing `&`/`<`/`>` (common in generated passwords, connection strings,
basic-auth blobs) was not found in the marshaled text and was **journaled forever**.
Fix: `kernel/bus/bus.go` now marshals the payload with a `json.Encoder` +
`SetEscapeHTML(false)` (trailing newline trimmed so the stored bytes are identical
in form), keeping those characters literal so the scrubber sees the real value.
Valid JSON either way; existing journaled events are unaffected (their bytes were
already hashed).

### High — base64/OAuth tokens leaked entirely via a char-class gap
The `sk-` (`[A-Za-z0-9_-]`) and `bearer` (`[A-Za-z0-9._-]`) classes excluded `+`,
`/`, `=` — standard base64 / many OAuth tokens. Because the patterns require
`{20,}` *contiguous* allowed chars, a token whose run before a `+`/`/` was shorter
than 20 produced **no match at all** — the whole secret leaked (e.g. a Google
`ya29.…` access token, a standard-base64 bearer). Fix: both classes now include
`._+/=-`, so the token matches whole.

### High — missing patterns for common formats
Patterns are the only net for secrets the daemon was never told about. Added:
- **JWT** — `eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+` (header.payload.sig;
  both segments begin `eyJ`, the base64url of `{"`). JWTs routinely carry bearer
  credentials in logged HTTP traffic.
- **GitHub fine-grained PAT** — `github_pat_[A-Za-z0-9_]{30,}` (only classic
  `ghp_`/`gho_`/… were covered).
- Widened the Google-key quantifier `{35}` → `{35,}` so a longer key variant isn't
  left with an unredacted tail.

## Deliberately not changed (this milestone)
- **`[]byte`-payload base64 (Critical, structural)** — a secret in a `[]byte`
  payload field is base64-encoded by `json.Marshal` before the scrubber runs, so it
  evades both literals and patterns. The robust fix is *structural* redaction (walk
  the payload and scrub values before marshaling, base64-decoding `[]byte`), which
  is a larger architectural change tracked as a follow-up. The real payloads in this
  codebase are `map[string]any` of strings/numbers, not `[]byte`, so the exposure is
  latent rather than active.
- **Bare 40-char AWS secret-key pattern** — too false-positive-prone without context
  (any 40-char base64 string), and such keys are configured creds already caught by
  the literal scrubber. Skipped intentionally.
- **Unicode NFC/NFD literal normalization** and **streaming-chunk redaction** —
  lower-likelihood, noted by the review for future consideration.

## Tests (+2, all passing)
- `kernel/redact/redact_test.go::TestPatterns_M170` — base64-bearing `sk-`/`bearer`
  tokens (incl. a Google `ya29.…`), a JWT, and a `github_pat_` are all redacted
  (each leaked entirely before the fix).
- `kernel/bus/bus_test.go::TestRedactor_LiteralWithHTMLChars` — a configured literal
  secret containing `&`/`<`/`>`, embedded in a connection-string payload, is scrubbed
  before journaling (returned + persisted), and neither the raw nor the
  HTML-escaped fragment survives; the redacted event still hash-verifies.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1548 tests** (was 1546; +2), 61 packages
  (the `SetEscapeHTML` change is on the journal-write hot path; the full suite
  confirms no regression).

## Result
Three real secret-leak paths into the permanent journal are closed: secrets with
`&`/`<`/`>`, base64/OAuth tokens, and JWT/`github_pat_` formats are now redacted.
The most security-critical chokepoint in the system is materially harder to slip a
credential past.
