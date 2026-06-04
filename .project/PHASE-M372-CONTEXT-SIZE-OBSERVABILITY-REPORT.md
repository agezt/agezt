# M372 — Record assembled context size per LLM call (SPEC-10 §3.5)

## SPEC audit (read-vs-code)
SPEC-10 §3.5 (Context observability) requires:

> Every LLM call records **exactly what was sent**: which tokens came from which
> source (system/skill/memory/turn/artifact), what was compressed, the total
> token count, and the cost. This is journaled and surfaced in the UI's **context
> inspector** … Most "why did it answer that?" questions are really "what was in
> its context?" — now answerable.

**Verified gap:** the agent loop publishes an `llm.request` event each iteration,
but it recorded only `iter`, `messages` (count), `model`, `tools` (count) — NOT
the size of the assembled context or where it came from. So the §3.5 foundation
("how big was the context, and from which source") was absent, and the context
inspector had nothing to render. (Full §3 context *management* — budgeting,
tiered assembly, compression — is a large separate feature; this milestone
delivers the observability foundation, not compression.)

## What
- **`kernel/agent/agent.go`** — new pure `contextSize(system, messages)` returns
  the total character count and a per-role breakdown (system/user/assistant/tool):
  each message's text content plus its tool-call argument JSON, plus the
  separately-sent system prompt (`CompletionRequest.System`, which is not in the
  message list but still occupies the window). Image attachments are excluded —
  a separate (vision) modality, not text context. The `llm.request` event now
  carries `context_chars` and `context_by_role`.
- Characters are an honest, deterministic, provider-agnostic proxy for context
  weight (~4 chars/token); the goal is relative visibility into context bloat and
  its source, not token billing — exact tokens land on `llm.response` from the
  provider's real usage (M282/M289).

## Verification
- **Unit (white-box, `context_internal_test.go`, 2):** per-role accounting
  including system prompt and tool-call args, image exclusion, exact totals;
  empty input → zero total + empty map.
- **Integration (black-box, `agent_test.go`):** a real `agent.Run` with a system
  prompt + task; the journaled `llm.request` event carries
  `context_by_role[system]==len(system)`, `[user]==len(task)`, and
  `context_chars==their sum`.
- **Live daemon demo:** a run with `AGEZT_SYSTEM_PROMPT="You are Agezt."` (14) and
  task "17 times 23" (11) journaled
  `context_by_role":{"system":14,"user":11}` in the live journal segment.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2131** passing (was 2128; +3), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, operator-visible).

## Scope notes
- This is the recording foundation for SPEC-07's **context inspector** UI and a
  follow-up: a web-UI run-detail panel that renders `context_by_role` per
  llm.request (the goal's priority-B "web UI context-inspector surface"). Logged
  for next.md, not built this turn.
- Full SPEC-10 §3 context *management* (budgeting/tiered-assembly/compression
  with journaled drops, §3.1–§3.4) remains a large feature — recorded honestly,
  not closed. The loop today sends the full accumulated history (bounded by
  MaxIter=25 + per-run cost cap + 64KiB tool-output cap, but not window-aware).
- SPEC-10 capability leverage (vision/JSON/caching/reasoning/streaming, §2) was
  built across prior sessions (M241-325) and is solid.
