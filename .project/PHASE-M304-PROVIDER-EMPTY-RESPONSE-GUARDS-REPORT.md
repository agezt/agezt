# M304 — Regression tests lock in the provider empty-response guards

## Why
Continuation of the M303 audit pass (same lens: untrusted/degraded bytes meeting
a primitive that can panic), this time on the **provider HTTP response parsing** —
the genuinely untrusted, *non*-hash-protected boundary where bytes from an
external API (or a flaky proxy that truncates the body) meet our JSON decoder and
a `[0]` index.

The classic LLM-client panic is assuming the response array (`choices` /
`candidates`) is non-empty and indexing `[0]`. I swept all eleven `[0]` index
sites across every provider (non-streaming completion *and* streaming frame
handlers) and confirmed **every one is already guarded** — either a `len(...) == 0`
check that returns a clean error, or, in the OpenAI streaming tool-call
assembler, a `map[int]` keyed by the provider-supplied `index` (a map key never
panics, sidestepping the slice-index bug entirely). The audit perimeter is solid.

But the audit also found that four of those load-bearing guards had **no
regression test**: a future refactor that dropped a `len == 0` check would
silently reintroduce a crash on a malformed/empty response, and nothing would
catch it. Only `bedrock/mistral` had a test (`TestComplete_MistralEmptyChoicesErrors`).

This milestone turns the audit findings into durable guards: a focused
"empty array → clean error, never panic" test for each of the four uncovered
providers, driven end-to-end through the public `Complete` via an `httptest`
server (matching the existing Mistral convention). Because the tests run with no
`recover`, a reintroduced panic fails them as loudly as a nil error.

## What — new regression tests (no production code changed)
- **`plugins/providers/openai/empty_response_test.go`** —
  `TestComplete_EmptyChoicesErrorsNotPanic`: server returns `{"choices":[]}`,
  asserts a clean error mentioning "no choices".
- **`plugins/providers/google/empty_response_test.go`** —
  `TestComplete_EmptyCandidatesErrorsNotPanic`: `{"candidates":[]}` →
  "no candidates" (Gemini legitimately returns no candidates on a safety block).
- **`plugins/providers/vertex/empty_response_test.go`** —
  `TestComplete_EmptyCandidatesErrorsNotPanic`: two servers (OAuth token +
  generateContent), the latter `{"candidates":[]}` → "no candidates" (the
  Gemini-on-Vertex path; reuses the package's `generateTestSAJSON` helper).
- **`plugins/providers/bedrock/ai21_empty_test.go`** —
  `TestComplete_AI21EmptyChoicesErrorsNotPanic`: `{"choices":[]}` → "no choices",
  mirroring the existing Mistral coverage for the AI21-Jamba decoder.

Each test asserts the guard's *specific* message (not just "an error"), proving
the `len == 0` branch is what executed — not an unrelated transport/decode error.

## Verification
- The four new tests **PASS**; full suite **1951** PASS (incl. subtests),
  63 packages, `go test ./...` exit 0.
- `gofmt -l` clean on all four new files; `go vet` clean on the four provider
  packages; `go.mod` / `go.sum` unchanged; no production code touched.

## Scope notes
- Pure test hardening — locks in existing, correct behaviour; no behaviour change,
  no new dependency.
- Audited and found solid in the same pass (no fix needed): all eleven `[0]`
  index sites (openai/google/vertex/bedrock-ai21/bedrock-mistral completion +
  openai/google/vertex streaming frames), and the OpenAI streaming tool-call
  assembler's `map[int]`-keyed accumulation (panic-proof against a hostile
  `index`). Also reconfirmed clean this pass: plugin-host stdio framing,
  mesh/peer response parsing, catalog `custom.json` load, journal/event replay
  (torn-line discard + chain verification), and cadence interval/window/daily
  walkers (MinInterval clamp, bounded walker loops — prior M196/M197 hardening).
