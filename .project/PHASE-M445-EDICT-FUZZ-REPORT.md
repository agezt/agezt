# M445 — Fuzz the trust-ladder decision path (edict)

## Context
Second fuzz target (after M444's redact), covering the other top
security-critical untrusted-input parser: `kernel/edict.Decide`. Its input is the
stringified tool argument, which the engine JSON-decodes, whitespace-collapses,
and matches against the hard-deny floor (`denyCandidates` →
`collectJSONStrings` → `stripPunctAdjacentWhitespace`) — the exact evasion surface
hardened reactively in M173 and M426. A panic here is a DoS; a hard-deny bypass
is a security-control failure.

## What was added
`kernel/edict/fuzz_test.go` — `FuzzDecide(capability, input)`, configuring the
fuzzed capability at `LevelAllow` over `DefaultHardDeny()` with `AskDeny` (the
worst case for the floor: a would-auto-allow capability, so the floor must still
bite). Three invariants:
1. **Never panics** — `Decide` / `DecideWithCeiling` over any (capability, input).
2. **Hard-deny floor is un-overridable** — if an input hard-denies, it stays
   `HardDenied` with `Decision=Deny` at *every* trust ceiling (L0..L4). This is
   the load-bearing trust-ladder invariant: a ceiling can tighten but never loosen
   the floor.
3. **Ceiling only tightens** — for ceilings `lo <= hi`, the decision at `lo` is
   never strictly less strict than at `hi` (with `AskDeny`, strictness = Deny).

Seeds include the canonical evasion vectors: `rm -rf /`, its JSON-wrapped form
`{"command":"rm -rf /"}`, whitespace-padded `rm  -rf   /`, and a fork bomb
`:(){ :|:& };:`.

## Verification
- **Seed run** (`go test ./kernel/edict/`): passes.
- **Fuzz run** (`go test -fuzz=^FuzzDecide$ -fuzztime=45s`): **2,650,667
  executions, PASS** — no panic, no hard-deny bypass, ceiling monotonicity holds.
  The fuzzer's 485 coverage-interesting inputs exercised the JSON-decode /
  whitespace-normalization evasion space without breaking the floor.
- **Negative-control note:** invariants 2 and 3 are themselves adversarial checks
  — any input the fuzzer found that overrode the floor or inverted the ceiling
  ordering would fail the run; none did across 2.65 M executions. (No corpus file
  written — a clean run records its corpus only in the build cache.)
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. CHANGELOG Security entry.

## Review status
The two highest-value untrusted-input security parsers — redaction (M444) and the
trust-ladder decision (M445) — are now fuzz-hardened. Further candidates
(control-plane request parse, journal torn-tail, provider SSE framing) remain
available if deeper coverage is wanted.
