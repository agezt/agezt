# M393 — Context budgeting / compaction (SPEC-10 §3)

## SPEC audit (read-vs-code)
SPEC-10 §3 (context management: tiered assembly + compression with journaled
drops) was the largest open offline item. Verified state: M372 added per-call
context-size OBSERVABILITY (`context_chars`/`context_by_role` on `llm.request`),
but the loop sent the FULL message history every call — no budget, no compaction.
A many-step run grows unbounded (cost + "lost in the middle" quality loss).

This milestone is the first concrete SPEC-10 §3 slice: a conservative,
auditable context budget that trims the loop's own context before each call.

## What
- **`kernel/agent/agent.go`** — `LoopConfig.ContextBudget` (chars; 0 = disabled) +
  `ContextProtectLast`. New pure `compactMessages(system, messages, budget,
  protectLast)`: when the assembled context exceeds the budget, it elides the
  OLDEST tool-result outputs to short stubs (`[tool output elided to fit context
  budget: N chars]`) until it fits or only protected messages remain. It NEVER
  touches the system prompt, any non-tool message, or the most recent
  `protectLast` messages (default 4); message count, roles, and tool-call ids are
  preserved (so provider tool_use/tool_result pairing stays valid); idempotent
  (a stub is not re-elided). The loop adopts the result and journals a
  `context.compacted` event (new event kind) with elided/reclaimed/before/after/
  budget.
- **`kernel/runtime/runtime.go`** — `Config.ContextBudget` → `LoopConfig`.
- **`cmd/agezt/main.go`** — `AGEZT_CONTEXT_BUDGET` (positive chars; unset = off) +
  config inventory entry.

## Verification
- **`kernel/agent/compact_internal_test.go`** (pure, 4 tests): budget 0 / under
  budget → no-op; over budget → elides oldest-first, gets under budget when
  possible, preserves count/roles/tool-call-ids, protects the tail; never elides
  user/assistant; idempotent.
- **`kernel/agent/context_budget_test.go`** (black-box loop, real bus + journal):
  three large tool rounds + a low budget → `context.compacted` journaled with
  elided>0, reclaimed>0, after<before, budget echoed; no budget → no compaction
  (full-history behaviour unchanged).
- **Negative control:** dropping the `messages = compacted` adoption → the
  integration test FAILs ("expected at least one context.compacted"); restored
  `agent.go` byte-identical.
- **Live demo** (daemon, `AGEZT_CONTEXT_BUDGET=150`, `AGEZT_DEMO_LOOP`): the run
  journaled `context.compacted` events — each `elided:1, reclaimed_chars:56,
  context_chars_before:390 → after:334` — the loop trimming its oldest tool
  output every iteration.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2198** passing (was 2192; +6). CHANGELOG (Added, user-visible).

## Scope notes
- Conservative v1 by design: it elides only OLD tool OUTPUTS (the bulk, already
  acted upon) — it does not summarise/compress with an LLM, re-order, or drop
  user/assistant turns. That keeps it correctness-safe and fully deterministic.
- Follow-on SPEC-10 §3 slices (recorded): LLM-summarisation of elided spans
  instead of a stub; protect-first-turns (currently protect-last only); a
  default budget tied to the model's context window from the catalog; and
  surfacing `context.compacted` in the web run-detail card.
