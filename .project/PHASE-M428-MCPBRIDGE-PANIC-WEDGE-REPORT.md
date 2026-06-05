# M428 — MCP bridge: response-path panic + duplicate-id wedge

## Context
Review of the MCP bridge (`plugins/external/mcpbridge`), which connects Agezt to
external MCP servers and processes their UNTRUSTED output. Framing (16 MiB cap), read
timeouts, malformed-JSON handling, and subprocess teardown were found sound. Two
genuine bugs in the JSON-RPC response/death path (`mcp.go`).

## Fixes

### HIGH — send-on-closed-channel panic crashes the bridge
`onResponse` looked up the pending channel under the mutex, released it, then sent on
the (unlocked) channel. `markDead` (called from a *different* goroutine — `run()`'s
`defer close()`, the handshake-error path) closed every pending channel under the lock.
Interleaving: the read goroutine reads `ch` (ok), releases the lock; `markDead`
concurrently closes `ch`; the read goroutine then `ch <- resp` on a **closed** channel
→ `panic: send on closed channel`, in the read goroutine where nothing recovers — the
whole bridge process dies. A server that replies and then drops its connection while
teardown runs (or just crashes-on-reply) triggers it.

Fix: a shared `done chan struct{}` closed once by `markDead`; the per-call channels are
**never** closed (markDead just deletes the map entries). `call` now also selects on
`<-m.done` to wake when the connection dies. A late send from the read goroutine then
lands harmlessly in the (never-closed) cap-1 buffer.

### MEDIUM — duplicate response id wedges the read loop
`onResponse` did a *blocking* send on the cap-1 pending channel. A server sending two
responses with the same id filled the buffer on the first and **blocked forever** on the
second — and since `onResponse` runs synchronously on the transport read goroutine, that
goroutine stalled, so no further frames (notifications, the death signal) were processed
and every subsequent call only failed via its own ctx timeout. One crafted frame
degraded the bridge to "every call times out."

Fix: the send is now non-blocking (`select { case ch <- resp: default: }`) — a duplicate
or late response is dropped instead of stalling the read loop.

## Verification
- **`plugins/external/mcpbridge/mcp_m428_test.go`** (white-box, package `main`):
  - `TestOnResponse_DuplicateIDDoesNotBlock`: two responses for one id complete
    promptly (no block).
  - `TestMarkDead_KeepsChannelsOpenAndSignalsDone`: `markDead` closes `done` and leaves
    the per-call channel open (a subsequent send does not panic).
  - **Negative controls:** reverting `onResponse` to a blocking send → the duplicate-id
    test times out; reverting `markDead` to close the channels → the markDead test
    panics on the post-death send. Both FAIL; restored byte-identical.
- The existing black-box bridge suite (list/invoke/resources/namespacing) still passes.
- **Gate:** `gofmt -l` clean on the edited files, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2287** passing (was 2285; +2).
  CHANGELOG Reliability entry.

## Notes
The bridge is a separate subprocess the daemon's plugin host supervises (and now reaps
cleanly, M422), so the crash blast radius was the bridge (losing its MCP tools), not the
daemon — still a real availability bug for any MCP-backed deployment. Two review LOWs are
deferred: the SSE `endpoint` URL is followed cross-origin (operator already trusts the
server URL), and a self-crashed stdio child is reaped only at `close()` (bounded by the
short-lived bridge process). A pre-existing gofmt deviation in `main.go`/`sse_transport.go`
(unrelated to this change) is addressed separately.
