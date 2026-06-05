# M436 — AWS assume-role duration: guard negative/malformed

## Context
Security-relevant review of the daemon entrypoint/wiring (`cmd/agezt/main.go`,
3315 lines; `cmd/agezt/awschain.go`, the AWS credential chain). The wiring is
**sound** — reported back essentially clean:

- **0.0.0.0 invariant HOLDS.** Every HTTP surface (control plane, web UI,
  OpenAI-compat, REST, Slack/Discord/webhook inbound) is off by default and
  requires an explicit operator-supplied `host:port`; there is no implicit
  `:port` / 0.0.0.0 derivation anywhere. Empty addr → the server is simply not
  started (early return), never a wildcard bind.
- **Tokens:** all three HTTP tokens use `crypto/rand` (32 bytes hex); a
  `rand.Read` error disables the server rather than starting it unauthenticated;
  the token mint precedes `net.Listen` in every builder, so **no server ever
  starts without a token**. The control-plane token is not printed.
- **Env parsing** is consistently defensive (durations/ints guard `>0`/`>=0`,
  booleans treat unexpected values as OFF, `0` is explicit-disable not
  limit-removal). **Shutdown** is bounded (ctx-cancel + `Shutdown(3s)` per server,
  bounded drain). The standing-order fire goroutine has a `recover()` backstop.
- **awschain.go** ordering is correct (vault → env → SSO → AssumeRole →
  WebIdentity-before-default → default chain); no credential value leaks into the
  banner; the AssumeRole sub-chain is built explicitly to avoid mutex recursion.

One LOW finding remained.

## The bug
`cmd/agezt/awschain.go`: the STS AssumeRole session duration was parsed as

    if n, err := strconv.Atoi(v); err == nil { duration = n }

with **no `>0` guard** — the lone duration/int parse in the daemon wiring that
doesn't have one (every parse in `main.go` guards `>0`). `kernel/creds/sts.go`
substitutes the AWS default (3600 s) only for an **exact** `0`
(`if duration == 0 { duration = 3600 }`), so a negative value (a typo'd
`AGEZT_AWS_ASSUME_ROLE_DURATION_SECONDS=-3600`) flowed straight through to the
STS `DurationSeconds` form field and was rejected with a `ValidationError` at the
first credential resolution — a runtime failure of the entire AWS credential
chain (and, on the post-reload path, a silent degradation) instead of a graceful
fallback to the default.

## The fix
Extracted the parse into `parseAssumeRoleDurationSeconds(v string) int`, which
applies the `>0` guard: a missing, malformed, zero, or negative value returns 0,
which `kernel/creds` maps to the AWS default. Ordinary positive values are
unchanged. (Extracting it also makes the boundary unit-testable — the duration
isn't surfaced in the chain description otherwise.)

## Verification
- **`cmd/agezt/awschain_test.go`** `TestParseAssumeRoleDurationSeconds`: table —
  `""`/`"0"`/`"-5"`/`"-3600"`/`"abc"`/`"12.5"` → 0; `"3600"`/`"900"`/`"  1200  "`
  (trimmed) → their value.
  - **Negative control:** drop `&& n > 0` → the `-5` and `-3600` cases FAIL
    (parsed through verbatim). Restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2299** passing (was 2298;
  +1), `go test ./...` exit 0. CHANGELOG Reliability entry.

## Deferred (documented, not fixed)
- LOW: the web/openai/rest tokens are printed to the daemon's stdout boot banner
  (by design, operator-facing). Not written to disk by this code; worth an
  operator doc note that the daemon's stdout is sensitive when an HTTP surface is
  enabled. No code change.
- Design note: the HTTP surfaces are plaintext-only (no `ListenAndServeTLS`);
  non-loopback exposure is an explicit operator choice gated by a banner WARNING.
  TLS would need to become first-class if non-loopback is ever a supported mode.

## Review status
The daemon entrypoint/wiring and AWS credential chain are reviewed and sound.
