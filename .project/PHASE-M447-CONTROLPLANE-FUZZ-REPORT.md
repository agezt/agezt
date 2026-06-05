# M447 — Fuzz the control-plane pre-auth request parse

## Context
Fourth fuzz target, covering the highest-EXPOSURE untrusted-input surface: the
control plane's read+parse path runs BEFORE authentication (the token is inside
the request line), so any local client that can reach the loopback port feeds it
bytes pre-auth. The custom code is `readBoundedLine` (the bounded request reader,
M188), followed by a JSON unmarshal into `Request`.

## What was added
`kernel/controlplane/fuzz_test.go` — `FuzzRequestParse(data)` drives
`readBoundedLine` then the `Request` unmarshal exactly as `handleConn` does.
Invariants:
1. `readBoundedLine` never panics and never returns more than `max` bytes — a
   no-newline or oversized stream is bounded (the M188 pre-auth-OOM guard), not an
   allocation runaway.
2. On a complete line, `json.Unmarshal` into `Request` never panics.

Seeds include a valid request, a request with a wrong-typed arg
(`{"args":{"tenant":123}}`, exercising the later comma-ok type assertion's input),
an un-terminated stream, a bare newline, and raw binary.

## Verification
- **Seed run** (`go test ./kernel/controlplane/`): passes.
- **Fuzz run** (`go test -fuzz=^FuzzRequestParse$ -fuzztime=40s`): **7,900,739
  executions, PASS** — no panic, the byte cap held on every input. (Very high exec
  rate — pure in-memory parse.)
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Security entry.

## Review status
Four fuzzers now cover the daemon's untrusted/corrupt-input parse surfaces:
- secret redaction — credential-leak boundary (M444)
- trust-ladder decision — security policy / hard-deny floor (M445)
- journal reopen — data-integrity / corrupt-segment resilience (M446)
- control-plane request parse — the pre-auth network surface (M447)

The tree went from zero fuzz tests to covering every primary untrusted-input
parser, each verified clean across millions of executions.
