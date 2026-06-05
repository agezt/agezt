# M434 — acpagent: make the timeout real + bound teardown

## Context
Follow-up to the M433 plugin-tool review. Two findings in the ACP-agent bridge
tool (`plugins/tools/acpagent/acpagent.go`), which spawns an external ACP agent
as a subprocess and drives it over stdio JSON-RPC.

## The bugs
1. **MED — the context timeout was effectively dead.** `Invoke` wraps the call
   in `context.WithTimeout` (5 min default), but the agent is spawned with
   `exec.Command`, *not* `CommandContext`, and teardown happens only in the
   deferred `tr.close()` — which runs *after* `client.Prompt` (and
   `Initialize`/`NewSession`) returns. Those calls block inside a `bufio.Scanner`
   read on the child's stdout; the ACP client's `ctx.Err()` check only runs at
   the top of each loop iteration and cannot interrupt a parked read. So a
   silent or wedged external agent (deadlocked, waiting on its own I/O, or simply
   never replying) produces no stdout, and `Invoke` blocks indefinitely — the
   timeout never fires because nothing acts on the cancellation. The whole agent
   run that called the tool wedges.
2. **LOW — unbounded post-kill wait in `close()`.** After the 5 s graceful
   grace, `close()` did `Kill()` then a blocking `return <-done`. If the child
   has un-reaped descendants holding the stdout pipe, or `Wait()` otherwise
   stalls, this pins the caller and leaks the `done` goroutine.

## The fix
1. After dialing, `Invoke` starts a watcher goroutine that calls `tr.close()`
   when `ctx` fires (timeout or caller cancel). Closing the transport closes
   stdin and — if the child doesn't exit gracefully — kills it, which closes the
   child's stdout and unblocks the parked scanner read, so `Prompt` returns and
   `Invoke` exits within the teardown bound. The watcher always wakes (the
   deferred `cancel()` fires on return) and then exits, so it does not leak.
2. `spawnAgent`'s `close` is now wrapped in `sync.Once` (idempotent — the Invoke
   deferred close and the watcher may both call it) and the **post-kill wait is
   bounded** by a second 5 s timeout, after which it returns an error rather than
   blocking forever.

## Verification
- **`plugins/tools/acpagent/cancel_test.go`** (white-box):
  - `TestACPAgent_ContextTimeoutUnblocksWedgedAgent`: a fake transport whose
    stdout read blocks until `close()`; the Tool's `Timeout` is 100 ms. The test
    asserts `Invoke` returns well within a 5 s guard (the watcher tore the wedged
    read down).
  - `TestACPAgent_CloseIsIdempotent`: calling `close()` twice neither panics nor
    blocks.
  - **Negative control:** replace `<-ctx.Done()` with a never-firing channel →
    `TestACPAgent_ContextTimeoutUnblocksWedgedAgent` FAILs at the 5 s guard
    ("Invoke wedged well past its 100ms timeout"). Restored.
- **Gate:** staged (LF-normalized) blobs gofmt-clean (working-copy `gofmt -l`
  flag was the CRLF artifact — verified via `git show :file | gofmt -l`),
  `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod`/`go.sum` unchanged.
  Full suite **2296** passing (was 2294; +2), `go test ./...` exit 0. CHANGELOG
  Reliability entry.

## Review status
This closes the M433/M434 plugin-tool review (SDK, peer, coding, notify,
acpagent). The previously-deferred "acp.Client read not promptly
ctx-cancellable" item (noted in M423) is now resolved at the tool layer.
