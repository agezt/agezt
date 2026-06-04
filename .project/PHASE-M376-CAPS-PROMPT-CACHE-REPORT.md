# M376 — Advertise prompt-caching capability in `--caps` (SPEC-15 §1.2)

## SPEC audit (read-vs-code)
SPEC-15 §1.2 says the catalog records and advertises, per model: "modalities,
context window, **tool-use support, JSON-mode, prompt-caching, reasoning
support**, and pricing." §2.5/SPEC-10 §2 want capabilities advertised so the
Planner/Governor/operator know what a model can do.

**Verified vs `agt provider check --caps` (check.go `jsonCaps`):** tool-use,
reasoning, vision, attachments, and JSON-mode are all advertised (human + JSON).
But **prompt-caching was missing** — even though the catalog tracks per-model
cache pricing (`Cost.CacheRead`, used end-to-end by the M289-302 cache-aware
billing arc) and the Model has the data. A genuine SPEC-15 §1.2 advertising
parity gap (offline-verifiable, no new data needed).

## What
- **`kernel/catalog/types.go`** — new `(*Model).SupportsPromptCache()`: true iff
  the model carries a separate cache-read price
  (`Cost.CacheReadMicrocentsPerMTok() > 0`) — the exact signal Agezt's billing
  uses to bill cached prompt tokens at the cache-read rate. Nil-safe (free/local
  models with no Cost → false).
- **`cmd/agt/check.go`** — `jsonCaps` gains `prompt_cache bool`; populated in both
  the single-model (`runCheckCaps`) and all-providers (`runCheckCapsAll`) paths;
  rendered in `emitCapsHuman` as `prompt caching  : yes/no`. The `--all` matrix
  stays the curated at-a-glance subset (tools/vision/reason/context), consistent
  with how json_mode/attachments are detail-view-only.

## Verification
- **`kernel/catalog/catalog_test.go::TestModel_SupportsPromptCache`**: cache-read
  price → true; priced-but-no-cache → false; nil Cost (free/local) → false.
- **`cmd/agt/check_test.go::TestRunCheckCaps_PromptCacheAdvertised`** (real CLI
  function path): a cache-priced model's `--caps --json` reports
  `prompt_cache:true`, a plain model `false`, and the human output contains
  `prompt caching  : yes`.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2137** passing (was 2135; +2), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible).

## Scope notes
- SPEC-15 audited largely solid: §1 catalog sync (M-series) + local auto-discovery
  (M309) + size-cap (M346) + `catalog.synced`→changelog (M370); §1.2 capability
  advertising now complete (this); §2 tool-calling normalization (canonical
  ToolDef/ToolCall per provider, M279 dotted-name fix, all dialects); §3 ACP
  server+client (M256/257/322). Reasoning/JSON-mode/vision capability flags all
  present and surfaced.
- Honestly deferred (recorded, not closed): the capability-DEGRADATION journaling
  (§2.3/SPEC-10 §2 — a non-supporting provider silently prompt-falls-back without
  a journaled `degraded` event); the wiring is nuanced (the loop would need
  catalog/family lookup at the JSON-mode-request point) — a separate milestone.
