# M375 — Lock in plugin crash-isolation on tool invoke (SPEC-04 §0.2/§1.6)

## SPEC audit (read-vs-code)
SPEC-04 §0.2 (uniform error model) defines an `UNAVAILABLE` error class for a
plugin that is down, and §1.6/§2 promise crash isolation: "a crash in WhatsApp
never affects Telegram" / "the kernel keeps running … other plugins continue to
serve." The plugin host's own protocol doc (protocol.go) states it: "A plugin
that exits unexpectedly marks all its tools as unavailable; subsequent
invocations return a clear error. The kernel keeps running."

**Verified vs `kernel/plugin`:** implemented correctly — `remoteTool.Invoke`
checks `r.plugin.IsAlive()` first and, if the process has died, returns
`plugin: tool %q unavailable (plugin process is dead: <cause>)` WITHOUT touching
the dead process's pipe. `markDead` sets the `dead` atomic and closes pending
waiters. This is NOT a feature gap.

**The gap (test coverage, priority A resilience):** the existing tests cover
`Close`/`markDead`/`deathError` and their data-race safety, but nothing exercised
the actual contract at the tool-invocation boundary — that a dead plugin's
`remoteTool.Invoke` returns the clean "unavailable" error (surfacing the cause)
and does not panic, hang, or write to the dead pipe. That boundary is exactly
what keeps one plugin's crash from wedging a run or the daemon.

## What
Test-only, no production change. `kernel/plugin/deadtool_test.go`:
- **`TestRemoteTool_InvokeOnDeadPluginFailsCleanly`** — white-box: build a
  `Plugin` with nil cmd/stdin (so any write would panic), `markDead` it, wrap it
  in a `remoteTool`, and invoke. Runs the invoke in a goroutine with a 2s
  deadline (a regression that blocks fails loudly instead of hanging the suite),
  and asserts the error reports the tool "unavailable" AND surfaces the death
  cause ("simulated crash") — diagnosable, never silent.

## Verification
- **Negative control (proves the test bites):** removing the `IsAlive()`
  dead-check in `remoteTool.Invoke` made the test FAIL — the error fell through
  to a lower-level `plugin: dead: …` that lacks the "unavailable" contract
  message. Restored `host.go` byte-identical (git diff empty) → green.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2135** passing (was 2134; +1), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only).

## Scope notes
- SPEC-04 is largely the gRPC plugin-interface spec, much of it superseded by
  DECISIONS (B0: stdio + JSON-RPC, in-process first-party) or already covered:
  the seven interfaces exist as first-party impls (channels, providers, tools,
  memory, storage); pin/supply-chain (M-series, SPEC-06 §6); durable-before-ack
  (§6.4, locked M369); inbound-as-data security (§1.7, channel sig verify).
- Deliberate non-gaps recorded: the full `ErrorClass` taxonomy +
  `retryable`/`retry_after_ms` (§0.2) is superseded — the agentic loop feeds a
  tool error back to the model as a tool-result (B0d), which IS the retry
  mechanism, so the structured retry fields are not wired; the flat
  `Error`/`IsError` is the minimal contract (B0b). §3.6 artifacts
  (content-addressed file outputs) and §8.4 Chronos event/condition triggers are
  larger features — recorded for honest tracking, not closed here.
