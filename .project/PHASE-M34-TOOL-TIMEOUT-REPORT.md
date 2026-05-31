# Phase Report — Milestone M34 (Per-tool-call timeout)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/resilience). Seventh step on the resilience/observability
> axis (M28 → … → M34). M31 caps a whole run's wall-clock; M34 adds a finer cap —
> one tool call — that fails the call, not the run.

## Why

M31's per-run timeout is the right tool for "this run has gone on too long, kill
it". But the common failure is narrower: *one* tool call hangs (a shell command
that blocks, an HTTP tool dialing a dead host) while the run is otherwise healthy.
Killing the whole run there is wasteful — the model could recover by trying a
different command or skipping the step. M34 bounds each tool invocation
individually and, on overrun, feeds the model an error result instead of failing
the run.

## What shipped

- **`LoopConfig.ToolTimeout time.Duration` (`kernel/agent/agent.go`)** — when
  `> 0`, each `tool.Invoke` runs under `context.WithTimeout(ctx, ToolTimeout)`.
  The invocation result is classified by a four-way switch:
  - tool returned no error → use its result;
  - **parent run ctx is done** (`ctx.Err() != nil`) → propagate and fail the run
    (operator halt, M32 cancel, or the M31 per-run deadline — a run-level
    terminal, not a tool fault);
  - **the tool's own per-call deadline fired** (`toolCtx.Err() ==
    DeadlineExceeded`, captured *before* cancelling) → synthesise an `IsError`
    result `tool "X" exceeded its <dur> timeout` and continue the run;
  - any other tool error → the existing error-result behaviour, unchanged.
- **`runtime.Config.ToolTimeout`** threaded into `RunWith`'s and the sub-agent's
  `LoopConfig`, so the cap applies to lead runs and delegated sub-agents alike.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_TOOL_TIMEOUT=<duration>`;
  malformed = hard startup error; off by default; boot banner `tool timeout : …`.

## Design decisions

- **Tool overrun fails the call, not the run.** This is the whole point of M34 vs
  M31. Tool errors were already fed back to the model (the loop has always turned
  a tool's returned error into an `IsError` result); M34 adds a *deadline* to that
  path plus a clear message, so a hung tool becomes a recoverable error the model
  sees rather than an indefinite stall.
- **Distinguish run-cancel from tool-timeout by the parent context.** After
  `Invoke` returns an error, the loop checks `ctx.Err()` (the *run* context)
  first: if the run itself was cancelled/timed out, that wins and the run fails
  with the correct reason. Only when the run is healthy is the error attributed to
  the tool's own budget. `TestRun_RunCancelDuringToolFailsRun` pins this: a run
  cancelled while a tool is blocking fails with `context.Canceled` (→
  `task.failed reason=canceled`), not swallowed into a tool-error result.
- **Key off the tool context's deadline state, captured before cancel.** The
  timeout branch tests `toolCtx.Err() == DeadlineExceeded` rather than
  `errors.Is(invokeErr, …)`, because a tool may wrap its error without preserving
  the `DeadlineExceeded` sentinel (the warden returns a plain `"context deadline
  exceeded"` string). The state is captured *before* `toolCancel()` runs, since
  cancelling a not-yet-expired context would flip its `Err()` to `Canceled` and
  mask the distinction.
- **Applies to sub-agents too.** A delegated sub-agent runs the same loop; passing
  `ToolTimeout` through keeps the cap uniform so a sub-agent can't sidestep it.

## Tests

`kernel/agent/agent_test.go`:
- `TestRun_ToolTimeoutFeedsErrorNotFailure` — a tool that blocks past its 30 ms
  budget yields an `IsError` `tool.result` mentioning the timeout, **no**
  `task.failed`, and the run completes with the model's follow-up answer.
- `TestRun_FastToolUnderTimeoutUnaffected` — a tool that finishes within a
  generous budget runs normally.
- `TestRun_RunCancelDuringToolFailsRun` — a run cancelled while a tool is
  executing fails with `context.Canceled` (the tool timeout must not mask
  run-level cancellation).

Test count: **1226 → 1229**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (mock provider)

```
$ AGEZT_TOOL_TIMEOUT=1ns agezt …
  tool timeout     : 1ns per tool call (error result on overrun; run continues)

$ agt run "list the files"     # the mock invokes a shell `ls -la`
$ agt runs list 1
    started : … status: completed           duration: 8ms   iters: 2
# tool.result: error=true, "warden: start \"cmd\": context deadline exceeded"
```

With a sub-millisecond per-tool budget the shell call hit its deadline and came
back as an error `tool.result`, yet the **run still completed** (the model gave
its final answer after seeing the failed tool) — exactly the M34 contract: a tool
timeout degrades one call, it does not kill the run. (The synthesised
`exceeded its … timeout` message is exercised by the unit test with a tool that
returns `ctx.Err()` cleanly; the live shell tool surfaces the warden's own
deadline string, which the loop still classifies as an error result.)

## What's next

The resilience axis is now broad; remaining clean follow-ons:

1. **Cancel-on-disconnect** (MED) — tie an `agt run` client connection to its run
   so Ctrl-C cancels server-side (reuses M32's `CancelRun`); today `handleRun`'s
   ctx is the server root, so a disconnect leaves the run going.
2. **`agt runs list --since <dur>`** (LOW) — mirror M33's window on the list view.
3. **Tool-timeout observability** (LOW) — a dedicated `tool.timeout` event (or a
   reason tag on `tool.result`) so `agt runs stats` could surface a per-tool
   timeout rate, distinct from ordinary tool errors.
