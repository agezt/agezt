# PHASE M902 — Forge bias: prefer deterministic tools + self-improvement

**Status:** shipped
**Milestone:** M902 (first milestone after the M880–M901 reconcile; main was
unified so the runtime is editable again).
**Theme:** Backlog **#42** — bias agents toward code / tool-forge for
deterministic work and self-improvement.

## What shipped

A `forgeBias(tools)` section appended to each run's environment preamble
(`injectEnvironment` in `kernel/runtime/runtime.go`), right after the M848
capability briefing. Where the capability briefing says what the agent *can* do,
the forge bias says how to work *well*:

> **## Prefer deterministic tools — and improve your own**
> For work that must be exact, is repeatable, or you'll likely do again, reach
> for a tool instead of reasoning it out by hand each time:
> - Write a script (code_exec) so the result is deterministic, checkable, and
>   re-runnable — computation/parsing/transforms/exact-rule work belong in code.
> - When a one-off script recurs, forge it into a durable tool (tool_forge).
> - Check existing skills/tools before re-deriving; capture a working approach as
>   a reusable skill (skill op=learn).
> Treat each run as self-improvement: when you hit a capability gap, build the
> tool that closes it.

- **Tuned to the tools present:** each line emits only when its tool
  (`code_exec` / `tool_forge` / `skill`) is in the run's tool set; returns `""`
  when none are — no empty nudge.
- Reaches **every run path** because it rides `injectEnvironment` (the main loop,
  sub-agents, and workflow runs all call it).

## Verification

- `go build ./...` + linux/amd64 cross-build clean; `go vet ./kernel/runtime/`
  clean; my two files `gofmt`-clean. The full `kernel/runtime` suite passes
  (7.3s). New `TestForgeBias_TunedToTools` asserts: full set emits all three
  lines + the self-improvement close; absent tools drop their line; no relevant
  tool → empty.
- No new dep, no new env var, no config surface — a pure system-prompt addition,
  consistent with the M848 capability-briefing pattern.

## Notes
- Descriptive guidance, not a hard rail — it nudges the model toward determinism
  and compounding self-improvement (forge → reuse) without forbidding ad-hoc
  reasoning when that's the right call. Complements [[default-allow-posture]]:
  the agent already *may* do anything; this tells it *what's usually wiser*.
