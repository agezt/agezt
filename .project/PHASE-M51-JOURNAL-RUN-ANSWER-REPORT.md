# Phase Report — Milestone M51 (Journal the run answer)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 (event journal) × SPEC-12. Closes the longest-standing schema gap on
> the run-observability surface: the renderers expected a final answer that was
> never emitted.

## Why

`agt runs show` has a "final answer:" section, and M44's per-delegation outcome
was meant to show a sub-agent's result — but neither worked for real runs. The
agent loop journaled `task.completed` as `{iters, chars, stopped}` and
`llm.response` as `{usage, text_chars, …}` — the message *body* was never on the
wire. The renderers read `payload.message.content` from `llm.response`, which the
daemon doesn't populate, so the answer section was permanently empty (the code was
aspirational — M44's report flagged exactly this). An operator could see that a
run completed, how long it took, and what it cost (M50), but not *what it
actually said*. M51 journals the answer.

## What shipped

- **`answer` on `task.completed` (`kernel/agent/agent.go`)** — when the loop
  finishes, the final assistant text is added to the `task.completed` payload
  (alongside the existing `iters`/`chars`/`stopped`). The FULL answer is still
  returned to the caller unchanged; only the journaled copy is length-capped.
- **`truncateForJournal` + `maxJournaledAnswerRunes` (8192)** — caps the stored
  copy rune-safely with a `…[truncated]` marker, since the journal is append-only,
  hash-chained, and replayed on every projection rebuild — a multi-MB final
  message must not bloat it. The event's `chars` field preserves the true length.
- **Renderer prefers the journaled answer (`cmd/agt/runs.go`)** — `renderTaskArc`
  now reads `task.completed.answer` (authoritative, emitted once at end of run,
  after every `llm.response`) and falls back to the old `llm.response.message.content`
  path for pre-M51 runs. The "final answer:" section lights up for real runs with
  no other change.

## Design decisions

- **Carry it on `task.completed`, not `llm.response`.** `llm.response` fires every
  round — journaling its content would store every intermediate assistant turn
  (including pre-tool-call chatter), bloating the journal with text no one reads.
  `task.completed` fires once, at the end, carrying exactly the final answer. One
  answer per run, minimal growth.
- **Redaction is free.** The bus runs the M15 secret redactor over every durably
  published payload before journaling (`bus.go` marshals → `RedactBytes` → append).
  The answer rides that path, so secrets in a final message are scrubbed with no
  extra code — the same guarantee every other journaled field already has.
- **Cap the journaled copy, not the returned value.** The caller (control plane,
  sub-agent parent) still gets the complete answer; only the durable journal copy
  is bounded. So truncation never changes program behaviour — it only bounds
  on-disk growth, with `chars` recording what was elided.
- **Rune-safe, fast path first.** The byte-length check short-circuits the common
  short answer (no `[]rune` allocation); only an over-cap answer pays for the
  rune-boundary trim, which guarantees valid UTF-8 in the journal.

## Tests

- `kernel/agent/agent_test.go::TestRun_TaskCompletedCarriesAnswer` — a run's final
  text lands on `task.completed.answer`, and `chars` equals the true length.
- `TestRun_AnswerTruncatedInJournal` — a 20k-char answer is capped with the marker
  in the journal while the caller receives the full text; `chars` records 20000.
- `cmd/agt/runs_show_test.go::TestRenderTaskArc_FinalAnswerFromTaskCompleted` — the
  arc surfaces the answer from `task.completed` under "final answer:".
- The existing `TestRenderTaskArc_FinalAnswerSurfacesFromLlmResponse` (the fallback
  path) is unchanged and still passes.

Test count: **1278 → 1281**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline daemon)

```
$ agt run "what is this project?"
$ agt runs show <lead>
  …
  final answer:
    [offline-mock] I ran a directory listing via the shell tool. This project is
    Agezt — an open-source, MIT-licensed agentic operating system written in Go. …

$ grep -c '"kind":"task.completed".*"answer":' $AGEZT_HOME/journal/*.jsonl
  1
```

`agt runs show` now displays what the run actually produced — the answer section
that was empty since it was written.

## What's next

The run-arc view is now complete: intent → rounds → tools → delegations (with
outcome + cost) → final answer. Sharpest remaining frontiers:

1. **Sub-agent answer on the M44 `↳` line** (LOW-MED) — M51 journals every run's
   answer, including each sub-agent's. Fold it into `runEntry` + expose it on the
   runs-list row so `agt runs show <lead>` can show a one-line preview of each
   delegation's result inline, not just its status/cost. Builds directly on M50/M51.
2. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + a tenant-token allowlist; the last non-tenant-aware control surface.
3. **Boot-banner the delegation caps** (LOW) — echo the active depth / fan-out /
   spend ceilings at daemon startup, alongside the model-advisory / recovery banners.
4. **`agt runs stats` spend percentiles** (LOW) — extend the M47 spend aggregate
   with a per-run cost distribution (avg/p50/p95), mirroring the duration block.
