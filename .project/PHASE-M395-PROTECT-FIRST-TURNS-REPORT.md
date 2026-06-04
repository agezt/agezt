# M395 — Protect-first turns in context compaction (SPEC-10 §3)

## Context
M393 added budget compaction (elide the *oldest* tool outputs to stubs until the
assembled context fits); M394 auto-sized that budget from the model's catalog
window. Both elide strictly oldest-first, protecting only the tail
(`ContextProtectLast`). But the earliest tool results are often the run's
*grounding* — the directory listing, the file read, the spec lookup that framed
the whole task. Dropping those first can lose the thread. SPEC-10 §3's intent is
context *management*, not just a sliding window; protecting the head is the
standard "keep both ends, drop the middle" refinement.

## What
- **`kernel/agent`** — `compactMessages` takes a new `protectFirst` parameter:
  indices `[0, protectFirst)` are shielded from elision (in addition to the
  system prompt, every non-tool message, and the trailing `protectLast`). Elision
  now starts at `start := protectFirst` and walks forward through the elidable
  middle. New `LoopConfig.ContextProtectFirst` and `DefaultContextProtectFirst`
  (= 0, opt-in — existing setups keep byte-identical behaviour).
- **`kernel/runtime`** — `Config.ContextProtectFirst`, wired into the per-run
  `LoopConfig`.
- **`cmd/agezt/main.go`** — `AGEZT_CONTEXT_PROTECT_FIRST=<n>` (non-negative int;
  anything else warns and is ignored) + config inventory entry.

## Verification
- **`kernel/agent/compact_internal_test.go`**
  `TestCompactMessages_ProtectsFirstGrounding`: a 5-tool-pair transcript with a
  budget that forces dropping a few outputs. `protectFirst=0` elides the oldest
  tool output (index 2); `protectFirst=3` leaves index 2 whole (1000 chars
  intact) and instead elides the next-oldest unprotected output (index 4) — with
  roles/tool-call-ids still aligned. Existing M393/M394 tests updated to pass
  `protectFirst=0` (behaviour unchanged).
- **`kernel/runtime/context_budget_test.go`**
  `TestRun_ContextProtectFirstPlumbsThrough` (real kernel + journal): same tight
  budget compacts with no first-protection (control) but produces **zero**
  `context.compacted` events when `ContextProtectFirst=100` shields every
  elidable message — proving the Config field reaches the loop.
- **Negative controls (both bite, both restored byte-identical):**
  (1) `start := protectFirst` → `start := 0` in `compactMessages` → the pure
  protect-first test FAILs (index-2 grounding elided to 54 chars).
  (2) `ContextProtectFirst: k.cfg.ContextProtectFirst` → `0` in the runtime
  LoopConfig → the runtime plumb test FAILs (compacted=1 instead of 0).
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged.
  `TestConfigEnvVars_CoversCmdAgeztReads` green (new env var inventoried). Full
  suite **2203** passing (was 2201; +2). CHANGELOG (Added, user-visible env var).

## Scope notes
- SPEC-10 §3 now: observability (M372), compaction (M393), auto-sizing (M394),
  protect-first grounding (M395). Remaining slices: LLM-summarise elided spans
  (vs the size stub) and surfacing `context.compacted` in the web run-detail card.
