# M398 — Abstractive LLM-summary of elided tool outputs (SPEC-10 §3, final slice)

## Context
M397 gave an elided tool output a deterministic *extractive* head-snippet stub.
SPEC-10 §3's named follow-up is to *summarise* elided spans — replace the head
snippet with a model-written one-liner so the model keeps the **meaning** of the
dropped output, not just its first characters. This is the abstractive half and
the last named offline-doable SPEC-10 §3 item. It costs an extra provider call,
so the design resolves that tradeoff the only sensible way: **opt-in, off by
default** — existing setups are byte-for-byte unchanged.

## What
- **`kernel/agent`** — `LoopConfig.SummarizeElided func(ctx, toolOutput) (string,
  error)`. `compactMessages` takes a `summarize func(string) string`: when it
  returns non-empty the stub embeds `… · summary: "<one line>"` (bounded by
  `elidedSummaryChars = 160`); empty/absent falls back to the M397 head snippet.
  The loop wraps `cfg.SummarizeElided` in a closure with a **per-run cache keyed
  by output** (each distinct output summarised at most once) that swallows
  ctx/errors to `""` (always falls back, never fails the run). nil → zero extra
  calls. `elidedStubPrefix` is unchanged, so idempotency and the
  `len(stub) >= orig` guard still hold.
- **`kernel/runtime`** — `Config.ContextSummarize` (opt-in). When set and
  compaction is active for the run, `makeElidedSummarizer(provider, model, corr)`
  builds a bounded single-shot summariser: one user message ("Summarize this tool
  output in one short line…"), `MaxTokens = 64`, input capped at 8 KiB, routed
  through the run's own provider (the Governor) and attributed to the run via
  `corr` so the extra spend is billed honestly.
- **`cmd/agezt/main.go`** — `AGEZT_CONTEXT_SUMMARIZE=1` + config inventory entry.

## Verification
- **`kernel/agent/compact_internal_test.go`** `TestCompactMessages_AbstractiveSummary`:
  a supplied summarizer's text is embedded (`summary:` + the text, no `head:`);
  an empty summary falls back to the head snippet; the summarizer is consulted.
- **`kernel/runtime/context_budget_test.go`**
  `TestRun_ContextSummarizeEmbedsAbstractiveSummary` (capturing mock = the
  goal-allowed capturing-fake demo, since the offline demo mocks can't answer a
  summarise prompt): the mock returns a canned summary for the M398 prompt and a
  tool/loop response otherwise; the canned summary is observed flowing back into
  a later request's context — proving loop → makeElidedSummarizer → provider →
  stub → next request end to end.
- **Negative controls (both bite, both restored byte-identical):** (1) neutering
  the summary branch in `compactMessages` (`s := ""`) → the pure test FAILs (head
  snippet used, summarizer not consulted); (2) leaving `summarizeElided` nil in
  the runtime wiring → the integration test FAILs (no summary reaches context).
- **Daemon:** boots clean with `AGEZT_CONTEXT_BUDGET=auto AGEZT_CONTEXT_SUMMARIZE=1`
  (no parse warning). `TestConfigEnvVars_CoversCmdAgeztReads` green.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2208** passing (was 2206; +2). CHANGELOG (Added, user-visible env).

## Scope notes
- **SPEC-10 §3 context management is now COMPLETE**: observability (M372),
  compaction (M393), auto-sizing (M394), protect-first (M395), web surfacing
  (M396), extractive stub (M397), abstractive summary (M398). No SPEC-10 §3 slice
  remains.
