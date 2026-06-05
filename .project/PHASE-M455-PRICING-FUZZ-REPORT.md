# M455 — Fuzz the governor cost math (financial-control integrity)

## Context
The governor converts UNTRUSTED provider-reported token counts (from the LLM
provider's usage response — a buggy or hostile endpoint can report negative,
absurd, or overflow-inducing values) into microcents that drive billing AND the
daily spend ceiling. The load-bearing invariant is that cost is **never
negative**: a negative cost (from an integer overflow that wraps, or unhandled
negative token counts) would credit the ledger and effectively disable the
ceiling — a spend-cap bypass. The code computes with saturation (`saturatingAdd`
/ `saturatingMul`, overflow → `MaxInt64`, fail-closed). This fuzz proves the
invariant holds for any input.

## What was added
`kernel/governor/pricing_fuzz_internal_test.go` (white-box) — `FuzzCostMicrocents`
over `costMicrocents(model, in, out)` and `costMicrocentsCached(model, in, cached,
write, out)` with arbitrary model strings and token counts. Invariant: neither
function panics, and neither ever returns a negative cost. Seeds include a normal
case, an unknown model, all-`-1`, all-`MaxInt`, and `MinInt`/`MaxInt` mixes.

## Verification
- **Seed run**: passes.
- **Fuzz run** (`-fuzztime=25s`): **11,596,205** executions, PASS — no panic and
  no negative cost across arbitrary token counts (negative, `MaxInt`, `MinInt`) and
  model strings. The saturation/fail-closed arithmetic holds: an overflow yields
  `MaxInt64` (which trips the budget gate), never a wrapped negative that would
  bypass the ceiling.
- **Gate:** gofmt-clean, `go vet` clean, `go.mod`/`go.sum` unchanged, full suite
  exit 0. (Test coverage only, no behaviour change.)

## Fuzz coverage now (16 targets)
The 15 input-parser fuzzers (M444–M449, M453, M454) plus this numeric
financial-control fuzz. Every untrusted/corrupt/external-feed/binary parse surface
AND the untrusted-token cost math are now fuzz-hardened, clean across ~110 M+ total
executions.
