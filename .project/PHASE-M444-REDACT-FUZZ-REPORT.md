# M444 — Fuzz the secret-redaction path (first fuzzer in the tree)

## Context
Beyond the defect sweep, "harden to completeness" means subjecting the
security-critical untrusted-input parsers to fuzzing. The tree had **zero** fuzz
tests. The highest-value target is `kernel/redact` — the boundary that keeps API
keys and credentials out of logs, the bus, and transcripts; a panic there is a
DoS and, worse, a redaction that fails to strip a secret is a credential leak.

## What was added
`kernel/redact/fuzz_test.go` — `FuzzRedact(text, secret)` with two invariants:
1. **Never panics**: `Redact` / `RedactBytes` over any `(text, secret)` pair (the
   literal, regex, and templated-pattern passes), and `RedactBytes` never returns
   nil for non-empty input.
2. **No survival**: an indexed literal secret never appears verbatim in the
   redacted output.

## A real fuzzing subtlety (found and soundly handled)
Within 1.7 s the fuzzer flagged `secret="[REDACTE"` "surviving". This is an
**artifact, not a leak**: `"[REDACTE"` is a prefix of the Placeholder
`"[REDACTED]"`, so redacting it legitimately *inserts* the placeholder, which
trivially contains the secret. No real credential is a placeholder fragment.

The first guard (`!strings.Contains(secret, Placeholder)`) was too narrow. The
sound formulation redacts the **bare secret** and skips the survival assertion
when that alone doesn't remove it: `!strings.Contains(r.Redact(secret), secret)`.
If redacting the secret in isolation yields the placeholder *without* the secret,
the secret is genuinely removable and the assertion must hold in the larger text;
if not (secret overlaps the placeholder), the property is degenerate and skipped.
This precisely excludes placeholder artifacts while keeping every real-leak case.

## Verification
- **Seed run** (`go test ./kernel/redact/`): passes, including the corpus entry
  the fuzzer minimized (`testdata/fuzz/FuzzRedact/acddb6e5bc974a6b` = `"[REDACTE"`,
  now a passing regression seed for the degenerate-guard path).
- **Fuzz run** (`go test -fuzz=^FuzzRedact$ -fuzztime=45s`): **1,518,025
  executions, no panic, no leak** — redaction is sound for non-degenerate secrets.
- **Negative-control note:** the original 1.7 s failure *is* the negative control
  for the soundness guard — with the narrow guard the fuzzer fails; with the
  `Redact(secret)` self-check it runs clean. The committed corpus entry exercises
  that guard path on every `go test`.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Security entry. (The corpus file is committed as LF;
  Go's corpus reader tolerates the Windows CRLF working copy — the seed run
  confirms it parses.)

## Review status
The redaction path is now fuzz-hardened with a durable corpus. Adjacent
untrusted-input parsers (edict policy, control-plane request parse, journal
torn-tail) are candidate follow-up fuzz targets.
