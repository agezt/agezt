# Phase Report — Milestone M111 (`agt provider cost`)

> Status: **shipped** · Date: 2026-06-02 · SPEC-08 / cost transparency.

## Why

`agt plan cost` estimates a DAG plan's cost, but an operator just choosing a
model — or sanity-checking a bill — had no quick "what does this model cost?"
lookup. The pricing already lives in the synced catalog; this surfaces it
directly, plus a back-of-envelope estimate for a hypothetical token count.

## What shipped

- **`agt provider cost --model <id> [--input-tokens N] [--output-tokens N]
  [--json]`** — prints the model's input/output price per Mtok and, when token
  counts are given, the estimated cost. Reuses `CmdCatalogList` (no new
  control-plane command). Exit 1 when the model isn't in the catalog; a free/
  local (unpriced) model reports "no pricing" rather than $0.
- **Pure helpers** — `estimateCostMicrocents` (price × tokens / 1e6),
  `findModelCost` (locate a model + its provider in the catalog response),
  `commaInt` (thousands separators) — all unit-tested.

## Tests

- `TestEstimateCostMicrocents` — $3/$15-per-Mtok math: 1M in = $3, 200k out =
  $3, combined = $6, zero = $0.
- `TestFindModelCost` — finds a model + provider; misses an absent one.
- `TestCommaInt` — separator formatting across magnitudes.

Test count: **1371 → 1374**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt provider cost --model claude-sonnet-4-6
  input : $3.0000 / Mtok
  output: $15.0000 / Mtok
$ agt provider cost --model claude-sonnet-4-6 --input-tokens 1000000 --output-tokens 200000
  estimate for 1,000,000 in / 200,000 out: $6.0000
```
