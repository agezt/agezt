# M423 — Pulse observer panic containment (HIGH)

## Context
A review of the autonomous Pulse engine (periodic observers → salience → briefing) and
the ACP protocol. ACP framing/parsing, subprocess reaping, and pulse salience math /
concurrency were found clean. One HIGH daemon-crash bug surfaced (plus a LOW ACP
read-cancellation note — see below).

## The bug
`kernel/pulse/engine.go`: the heartbeat is a single resident goroutine (`Start`'s
`go func`). `tickOnce` called `obs.Poll(ctx)`, `e.sal.Score(...)` (which may call an
LLM provider), and `e.sink.Deliver(...)` (telegram/slack/discord/webhook/email)
inline, with **no `recover()` anywhere in the package**. A panic in any of them —
a buggy observer, a panicking provider (a common real failure the agent loop already
recovers at agent.go), or a misbehaving channel sink — propagated up the loop
goroutine with no recovering frame and terminated the whole process (control plane,
every channel, all in-flight runs). This is exactly the class `kernel/standing`
(`safeFire`, M413) and `kernel/cadence` (`fireOne`, M420) explicitly guard against;
Pulse was the one resident loop missing the backstop.

## The fix
`tickOnce` now polls each observer through `safePoll`, which wraps the poll + delta
pipeline in a `recover()` that journals the panic as a contained observer-delta error;
the periodic digest flush is wrapped by `safeFlushDigest`. Per-observer granularity
(like `safeFire`) means one bad observer doesn't abort the whole beat.

## Verification
- **`kernel/pulse/engine_test.go`** `TestTickOnce_ContainsObserverPanic`: an observer
  that panics on `Poll`, driven synchronously through `tickOnce`, does not propagate;
  the tick completes (`pulse.tick` journaled) and the panic is recorded as a contained
  `observer.delta` error.
  - **Negative control:** disabling `safePoll`'s `recover()` → the synchronous tick
    panics the test goroutine → FAIL. Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2278** passing (was 2277; +1). CHANGELOG
  Reliability entry.

## Other review finding (deferred — LOW, mitigated)
`kernel/acp/client.go`: `call` checks `ctx.Err()` only before the blocking
`scanMessage` read, so a peer that goes silent mid-call isn't promptly cancelled by a
ctx deadline. Mitigated in the wired path: `acpagent.go`'s teardown closes stdin then
kills the process after a 5s grace, which closes stdout and unblocks the read, so the
process is always reaped and the goroutine drains — the impact is a slow (not
permanent) cancel for the `acp_agent` tool, and only a standalone `acp.Client`
consumer would hang. Left as documented; the ACP server framing (8 MiB cap, no panic
on malformed JSON, serialized writes) and subprocess lifecycle were found clean.
