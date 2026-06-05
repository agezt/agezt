# M449 — Fuzz the provider streaming-response parsers

## Context
Sixth (final) fuzz milestone, closing the secondary untrusted-input surface named
at the prior checkpoint: the provider streaming (SSE/JSONL) parsers. These decode
the LLM provider's streamed response — semi-trusted (operator-chosen provider) but
exposed to a MITM or buggy proxy in front of the API. They were code-reviewed
clean in M431 ("all four parsers bounded, ErrTooLong surfaced, no panic"); this
adds the fuzz backstop.

## What was added
`fuzz_test.go` in each of the five white-box-testable provider packages —
`FuzzParseStream` over `parseStream(body, [model,] onChunk)`:
- openai, anthropic (`parseStream(body, onChunk)`)
- google, cohere, ollama (`parseStream(body, model, onChunk)`)

Invariant: `parseStream` never panics or hangs on arbitrary bytes — a malformed,
truncated, or hostile stream must yield a clean error, not crash the agent loop.
Seeds cover a well-formed frame, empty input, non-JSON data lines, a truncated
JSON frame, and raw binary.

(Vertex and Bedrock use external `_test` packages / binary AWS event-stream
framing and aren't reachable as a white-box `parseStream` fuzz; their decoders
were code-reviewed in M431/M432.)

## Verification
- **Seed runs**: all five pass.
- **Fuzz runs** (`-fuzztime=15s` each):
  - openai — **3,250,120** executions, PASS
  - anthropic — **3,543,091** executions, PASS
  - google — **3,546,244** executions, PASS
  - cohere — **3,707,736** executions, PASS
  - ollama — **3,267,352** executions, PASS
  ~17 M total executions, no panic and no hang across all five — the bounded
  scanner and per-frame handling hold on hostile input.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Reliability entry.

## Fuzz arc complete
The tree went from zero fuzz tests (before M444) to **13 fuzz targets** covering
every primary and secondary untrusted/corrupt-input parser:
- redaction (M444), trust-ladder (M445), journal reopen (M446),
  control-plane pre-auth parse (M447),
- inbound channel signature verify ×3 (M448),
- provider streaming parse ×5 (M449).
All verified clean across tens of millions of executions; the credential-leak,
security-policy, data-integrity, pre-auth-network, channel-authenticity, and
upstream-stream surfaces are now fuzz-hardened with durable regression guards.
