# M366 — Redact connection-string passwords (SPEC-06 §4)

## SPEC audit (read-vs-code)
SPEC-06 §4 (Secret handling) names the exact set a `RedactingFormatter` must
mask before anything enters the journal or reaches a provider:

> a `RedactingFormatter` masks secrets (API keys, tokens, auth headers,
> **connection-string passwords**, private-key blocks, phone numbers) in all
> logs and tool output … Short tokens fully masked; long tokens keep a
> recognizable prefix/suffix.

**Verified coverage vs `kernel/redact`:**
- API keys ✓ (openai/anthropic, aws, github×2, slack×2, telegram, groq, xai,
  perplexity, fireworks, google), JWT ✓, auth headers ✓ (bearer-token),
  private-key blocks ✓ (pem-private-key).
- **connection-string passwords — MISSING.** A DB URI like
  `postgres://user:PASSWORD@host` is a real credential that lands in tool output,
  error strings, and config echoes; none of the 13 token-format patterns match
  the password inside a URI. Genuine SPEC-06 §4 gap and a real leak vector
  (priority-A secret leakage).

## What
- **`kernel/redact/redact.go`** — new `templatedPattern` mechanism (a detector
  that preserves non-secret context via a regexp replacement template, run after
  the full-match patterns) and the first one: `connection-string-password`.
  RE2 has no lookbehind, so the `scheme://user:` prefix (group 1) and the `@`
  boundary (group 2) are captured and re-emitted; only the password between them
  is masked → `scheme://user:[REDACTED]@host` (§4's "keep a recognizable
  prefix/suffix"). The password class includes `@` so a raw un-encoded `@` in the
  password is fully masked (greedy backtrack to the last `@`); `\s` stays
  excluded so two space-separated URIs on one line are masked independently.
  Also reported by `MatchedCategories` (so `agt redact test` names it).

## Verification
- **`kernel/redact/redact_m366_test.go`** (4 tests): masking across
  Postgres/MySQL/MongoDB/AMQP/Redis (incl. empty-user and raw-`@`-in-password,
  asserting the secret is gone AND scheme/user/host survive); a no-false-positive
  set (host:port, userinfo-without-password, `@`-in-query, non-URI — all left
  unchanged); two-URIs-on-one-line independence; `MatchedCategories` reporting.
- All pre-existing redact tests still pass (the new mechanism is additive; the 13
  full-match patterns are untouched).
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2112** passing (was 2108; +4), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Security, user-visible).

## Scope notes
- **Phone numbers (also named in §4) deliberately NOT added.** The redact
  package's contract is "high-confidence secret *formats*… implausible as
  ordinary prose, so a full-match replacement is safe." Phone-number regexes are
  false-positive magnets (ports, IDs, ULIDs, timestamps, version strings in tool
  output) and would mangle legitimate numeric data. SPEC-06 §8 frames PII
  redaction as "configurable" (opt-in), not a default high-confidence pattern.
  Recorded as a deliberate exclusion, not an oversight — adding it as an opt-in
  configurable PII pass is a separate, lower-priority design.
- SPEC-06 §4 now fully covered for the secret-format targets; the remaining
  §-items (Warden isolation downgrade journaling §2.2, anomaly auto-halt §5) are
  separate audits for later turns.
