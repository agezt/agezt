# Phase Report — Milestone M44 (Per-delegation outcome on the lead's arc)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Fourth step on the multi-agent axis: from
> "see the delegation" (M41–M43) to "see how it turned out".

## Why

After M41–M43, `agt runs show <lead>` renders `delegated → <child> (task: …)` for
each sub-agent. But the line stopped at the *intent* — it didn't say whether the
delegation **succeeded**. To learn the outcome an operator had to copy the child
correlation and run `agt runs show <child>`. The single most useful fact about a
delegation — did it complete, fail, time out? — was one command away when it
should be inline.

## What shipped

- **Inline child outcome in `renderTaskArc` (`cmd/agt/runs.go`)** — the
  `subagent.spawned` case now, after the `delegated → <child>` line, prints the
  sub-agent's terminal status indented beneath it:
  `↳ completed (1 iters, 1ms)` / `↳ failed (timeout) (3 iters)` / `↳ running (… iters)`.
- **`childOutcome` + outcome map (`cmdRunsShow`)** — `cmdRunsShow` already fetches
  the full runs list (to find the lead's row); it now also indexes every run's
  summary by correlation and builds a `map[correlation]childOutcome`
  (status/reason/iters/duration) passed into `renderTaskArc`. **No extra
  round-trips, no server change** — the data was already on the wire.

## Design decisions

- **Reuse the already-fetched runs list.** The sub-agent's status/iters/duration
  are exactly the fields `collectRuns` already produces and `cmdRunsShow` already
  pulls. Indexing them by correlation is free; no new endpoint, no N+1.
- **Status, not answer — honestly.** The original aim included the child's answer
  text, but the event schema doesn't journal it: `llm.response` records
  `text_chars` and `usage`, not the message body (the existing "final answer:"
  rendering is likewise schema-aspirational and empty for real runs). Rather than
  ship answer-rendering that can never fire live — against this project's
  demo-gated culture — M44 surfaces the *outcome that is journaled* (status, iters,
  duration). That answers "did the delegation succeed?", which is the first-order
  question; the child's full arc is a `runs show <child>` away. `childOutcome`'s
  doc comment records the schema limitation for the next session.
- **Graceful when absent.** A sub-agent whose summary isn't in the fetched window
  (or a malformed outcome) simply renders the `delegated →` line without the
  inline status — never a crash or a blank `↳`.

## Tests

`cmd/agt/runs_show_test.go`:
- `TestRenderTaskArc_DelegationShowsChildOutcome` — a `subagent.spawned` event with
  a matching outcome renders `delegated → <child>`, `(task: …)`, and
  `↳ completed (1 iters, 42ms)`.
- `TestRenderTaskArc_DelegationNoOutcomeStillRenders` — with `nil` outcomes the
  spawn line still renders and no `↳` outcome line appears (graceful degradation).
- The four existing `renderTaskArc` tests were updated for the new signature
  (pass `nil` outcomes) and pass unchanged.

Test count: **1263 → 1265**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=1)

```
$ agt runs show <lead>
  correlation: run-…3M…
  intent     : describe this project
  …
  delegated → run-…DJ…  (task: summarize the kernel package layout)
    ↳ completed (1 iters, 1ms)
```

The lead's arc now shows, inline, that its sub-agent completed in one iteration —
the delegation outcome without a second command.

## What's next

The delegation-observability surface (link, backlink, tree, outcome) is now
operator-complete. The remaining multi-agent frontier is **governance**:

1. **Sub-agent budget/policy surfacing** (MED) — how `delegate` is gated by Edict,
   and whether sub-agents share the lead's governor budget/ceiling or get their own
   (`kernel/runtime/subagent.go` + governor). Deeper, more product value.
2. **`agt runs stats` delegation metrics** (LOW) — runs that delegated, avg
   sub-agents per run, max depth seen.
3. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + allowlist for tenant tokens.
4. **Journal the run answer** (MED) — if `task.completed` (or `llm.response`)
   carried the final text, M44 could show the sub-agent's answer inline and the
   "final answer:" arc section would work for real runs. Schema change — weigh
   against journal size / redaction.
