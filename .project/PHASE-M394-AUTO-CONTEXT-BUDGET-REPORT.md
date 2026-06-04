# M394 — Auto context budget from the model's catalog window (SPEC-10 §3 / SPEC-16 §3)

## Context
M393 added context compaction gated on a fixed `AGEZT_CONTEXT_BUDGET` char count.
SPEC-10 §3's intent is *automatic* context management; SPEC-16 §3 names a
`compress_at_fraction: 0.5`. The catalog already tracks each model's token window
(`Model.Limit.Context`), so the budget can size itself to the model instead of a
hand-tuned constant.

## What
- **`kernel/agent`** — `AutoContextBudgetChars(contextTokens)` = `tokens × 4 ×
  0.5` (chars/token × compress fraction); returns 0 for a non-positive window so
  the caller leaves compaction off rather than guessing. Constants
  `ContextCharsPerToken` (4) and `DefaultCompressFraction` (0.5) are exported.
- **`kernel/runtime`** — `Config.ContextBudgetAuto`. In `RunWith`, when no explicit
  `ContextBudget` is set and auto is on, derive the budget from the resolved
  model's catalog `Limit.Context`. An explicit budget always wins; an unknown
  model → 0 (off).
- **`cmd/agezt/main.go`** — `AGEZT_CONTEXT_BUDGET=auto` enables it (a positive int
  is still an explicit budget; anything else is off). No behaviour change for
  existing setups.

## Verification
- **`kernel/agent/compact_internal_test.go`** `TestAutoContextBudgetChars`:
  0/negative → 0; 8192 → 16384; 200000 → 400000.
- **`kernel/runtime/context_budget_test.go`** (real kernel + injected catalog +
  real journal): `TestRun_AutoContextBudgetFromCatalog` — a model with a 10-token
  window (→ 20-char budget) + three large tool rounds → `context.compacted`
  journaled; `TestRun_AutoBudgetOffForUnknownModel` — auto mode with a model the
  catalog doesn't know → no compaction (no guessing a window).
- **Negative control:** replacing the auto-derivation with `ctxBudget = 0` → the
  auto-budget test FAILs (no context.compacted); restored `runtime.go`
  byte-identical.
- **Daemon check:** `AGEZT_CONTEXT_BUDGET=auto` boots cleanly (no parse error).
  (A full live compaction demo needs a catalog-known model; the auto-picked mock
  has no catalog window, so the runtime integration test with an injected catalog
  is the end-to-end proof — the allowed offline/capturing-fake mode.)
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2201** passing (was 2198; +3). CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-10 §3 now: observability (M372), compaction (M393), auto-sizing from the
  model window (M394). Remaining slices: LLM-summarise elided spans (vs stub),
  protect-first-turns, and surfacing context.compacted in the web run-detail card.
