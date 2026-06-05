# M454 — Fuzz the Bedrock binary event-stream parser

## Context
The AWS Bedrock streaming response is `application/vnd.amazon.eventstream` — a
BINARY framing format, not SSE. `parseEventStream` / `readEventStreamMessage` /
`parseEventStreamHeaders` read attacker-influenceable length and offset fields
(`totalLen`, `headersLen`, per-header `valueLen`) with `binary.BigEndian` and
slice buffers by them. Binary length-prefixed parsers are the highest
panic/OOM-risk surface in the tree (out-of-bounds slicing, huge allocations from a
crafted length). It had hand-written guards (min/max frame size, `headersLen`
bound) but no fuzzing.

## What was added
`plugins/providers/bedrock/fuzz_test.go` (white-box `package bedrock`, alongside
the existing `bedrock_test` black-box tests) — `FuzzParseEventStream`: arbitrary
bytes → `parseEventStream` never panics, never OOMs, never hangs (a
malformed/truncated/hostile frame must yield a clean error). Seeds include the
smallest accepted frame shape, a huge-`totalLen` prelude, truncated preludes, and
non-event-stream bytes.

## Verification
- **Seed run**: passes.
- **Fuzz run** (`-fuzztime=30s`): **2,325,144** executions, PASS — no panic, no
  OOM, no hang across millions of crafted binary frames. The length guards
  (`totalLen` 16…16 MiB, `headersLen <= totalLen-16`) and the header walker hold
  against the fuzzer's attempts to overflow the slicing.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. (Test coverage only, no behaviour change.)

## Fuzz coverage now (15 targets)
Every untrusted/corrupt/external-feed input parser in the daemon is fuzz-hardened:
redaction, trust-ladder, journal reopen, control-plane pre-auth parse, 3× channel
signature verify, 5× provider SSE/JSONL stream parse, catalog feed parse,
openai-compat message content, and the Bedrock binary event-stream parse — clean
across ~100 M+ total executions. The binary parser (this milestone) was the
highest-risk remaining surface; it is now covered.
