# M446 — Fuzz the journal reopen/parse path (corrupt-segment resilience)

## Context
Third fuzz target. The journal is the daemon's source of truth; its reopen path
parses an on-disk JSONL segment that a crash (half-written line), bit-rot, or
tampering can leave corrupt. The parsing is custom code — `lastCompleteOffset`
(the M417 torn-tail truncation), the `scanCompleteLines` split func, segment
scanning, and the BLAKE3 hash-chain `Verify`. A corrupt journal must never crash
or hang the daemon on startup (an availability failure that would also block
recovery).

## What was added
`kernel/journal/fuzz_test.go` — `FuzzJournalOpen(data)` writes the fuzzer's bytes
as the first segment (`00000001.jsonl`), then `Open` + `Range` + `Tail` + `Verify`
+ `Head`. Invariant: nothing panics or hangs. `Open` may legitimately reject a
corrupt segment with an error (correct handling) or accept it with the torn tail
truncated; either way every path must terminate cleanly.

Seeds cover the realistic corruption shapes: empty, non-JSON, a well-formed
record, a torn mid-line tail, blank lines, and raw binary/NUL bytes.

## Verification
- **Seed run** (`go test ./kernel/journal/`): passes.
- **Fuzz run** (`go test -fuzz=^FuzzJournalOpen$ -fuzztime=45s`): **90,855
  executions, PASS** — no panic, no hang across corrupt-segment variations. (Lower
  exec rate than the in-memory fuzzers because each iteration does real file I/O +
  a full journal Open; still tens of thousands of distinct corrupt inputs.) A hang
  in the custom `scanCompleteLines` split func or torn-tail logic would have been
  flagged by the fuzzer's per-input deadline — none occurred.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Reliability entry.

## Review status
Three of the highest-value untrusted/corrupt-input parsers are now fuzz-hardened:
secret redaction (M444), the trust-ladder decision (M445), and journal reopen
(M446). The tree went from zero fuzz tests to covering its credential-leak,
security-policy, and data-integrity parse surfaces.
