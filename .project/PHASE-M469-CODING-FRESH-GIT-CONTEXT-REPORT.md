# M469 — Coding tool: capture partial work when the agent times out

## Context
The coding tool runs an external coding agent in an isolated git worktree, then
stages and diffs the result to return a proposed (unapplied) patch:

```go
agentOut, agentErr := t.run(ctx, wt, agentEnv, shell, shellArg, t.Cmd)
if out, err := t.run(ctx, wt, nil, "git", "add", "-A"); err != nil { ... }
diff, derr := t.run(ctx, wt, nil, "git", "diff", "--cached", "HEAD")
```

## The bug (LOW)
The post-agent `git add`/`git diff` reuse the request `ctx`. If the agent runs to
the request deadline (or `ctx` is otherwise cancelled), `exec.CommandContext` with
an already-cancelled context returns `context.DeadlineExceeded` **without running
git at all**. So a coding agent that timed out — but had already written useful
partial changes — yields `"git add failed: context deadline exceeded"` and the
partial diff is discarded, instead of returning the partial work plus the agent's
timeout note. The worktree-cleanup defer in the same function already uses a fresh
context for exactly this reason.

## The fix
Stage and diff on a fresh bounded context:

```go
gitCtx, cancelGit := context.WithTimeout(context.Background(), 30*time.Second)
defer cancelGit()
if out, err := t.run(gitCtx, wt, nil, "git", "add", "-A"); err != nil { ... }
diff, derr := t.run(gitCtx, wt, nil, "git", "diff", "--cached", "HEAD")
```

The agent's timeout is still surfaced — `agentErr` flows into `renderResult`, which
appends `[agent exited with error: …]`. So the model gets both the partial diff and
the timeout signal.

## Test + negative control
`plugins/tools/coding/coding_test.go`: `TestCoding_PostAgentGitUsesFreshContext` —
a fake `run` records `ctx.Err()` at the `git add`/`git diff` calls; `Invoke` is
called with an **already-cancelled** context (simulating a timed-out agent), and the
test asserts those git calls saw a non-cancelled context and still ran.

**Negative control:** forcing the git calls back onto the request ctx (`gitCtx =
ctx`) made them observe `context canceled` — the test FAILED with `git add ran on an
expired context (context canceled): a timed-out agent's partial work is discarded`.
Restored; test passes.

## Verification / gate
- `plugins/tools/coding` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
