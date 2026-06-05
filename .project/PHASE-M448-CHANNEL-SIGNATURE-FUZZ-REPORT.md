# M448 — Fuzz inbound channel signature verification (forgery resistance)

## Context
Fifth fuzz milestone, covering the inbound-channel authenticity gate — the
verification that decides whether an untrusted-internet POST is an authentic
command. A bypass here is forged-command / forged-interaction injection, the
highest-severity channel risk. Three independent verifiers:
- Slack: HMAC-SHA256 over the `v0:ts:body` scheme + freshness window.
- Discord: Ed25519 over `ts+body` + freshness window.
- Webhook: HMAC-SHA256 over the body (`sha256=<hex>`).

These were code-reviewed in M431 (inbound parse) but never fuzzed.

## What was added
`fuzz_test.go` in each channel package — `FuzzVerify` with the same three
invariants per verifier:
1. **Never panics** on any `(ts, sig, body)` (hex decode, timestamp parse, MAC).
2. **Correct signature accepted** — a signature computed from the configured
   secret/key over the body at a fresh timestamp verifies true.
3. **No forgery** — for every fuzzer-supplied signature that differs from the
   correct one, `verify` returns false. This is the load-bearing security
   property: no signature other than the authentic one is ever accepted.

Slack/Discord use the injectable `now` clock pinned to a fixed time so the
freshness window passes and the test exercises the MAC/Ed25519 comparison itself;
Discord uses a deterministic Ed25519 keypair from a fixed seed.

## Verification
- **Seed runs**: all three pass.
- **Fuzz runs** (`-fuzztime=25s` each):
  - webhook — **1,964,309** executions, PASS
  - slack — **1,942,480** executions, PASS
  - discord — **3,736,690** executions, PASS
  No panic and no forgery across ~7.6 M total executions — no fuzzer-supplied
  signature was ever accepted in place of the authentic one.
- **Negative-control note:** invariant 3 IS the adversarial check — any accepted
  forged signature fails the run; none did. Invariant 2 (correct sig accepted)
  guards against the inverse regression (a verifier that rejects everything would
  trivially pass invariant 3 but fail 2).
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Security entry.

## Review status
Fuzz coverage now spans the daemon's primary untrusted/corrupt-input parsers
(redaction M444, trust-ladder M445, journal M446, control-plane parse M447) AND
the three inbound-channel signature verifiers (M448) — the credential-leak,
security-policy, data-integrity, pre-auth-network, and channel-authenticity
surfaces, all verified clean across tens of millions of executions.
