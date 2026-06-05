# M472 — Scheduler: event-driven driver (remove the 1 ms busy-wait)

## Context
The plan executor's driver loop launches every ready node, then waits for at least
one to finish before re-polling readiness.

## The issue (perf)
It waited by polling:

```go
for {
    ready := pickReady()
    for _, id := range ready { wg.Add(1); go runNode(...) }
    mu.Lock(); inflight := len(started) - len(completed); mu.Unlock()
    if inflight == 0 { break }
    time.Sleep(time.Millisecond) // busy-wait
}
```

Correct, but while any node is in flight the driver spins: every ~1 ms it wakes,
takes `mu`, scans `started`/`completed`, and sleeps again — for the entire duration
of the longest in-flight node (a `LoopNode` can run for seconds-to-minutes). It also
caps scheduling latency at ~1 ms granularity. Not a correctness defect, but real CPU
and lock churn the author flagged as a workaround ("avoids touching wg internals").

## The change
A buffered completion channel replaces the poll:

- `done := make(chan struct{}, len(plan.Nodes))`.
- Each `runNode`, after recording its completion under `mu`, sends one `done`
  (buffered to the node count, so it never blocks; a late send after the driver has
  moved on is simply discarded).
- The driver blocks on `<-done` instead of sleeping, then re-polls.

No busy-wait, no latency floor. It can't deadlock: the driver only receives when
`inflight > 0`, which means at least one node is running and will send. Termination
is unchanged — it still breaks when `inflight == 0` (keyed off `started`/`completed`,
not the channel). Behaviour is otherwise identical.

## Test + negative control
`kernel/scheduler/scheduler_test.go`: `TestRun_DeepChainEventDrivenDriver` — a
64-node linear chain completes one node at a time, so the driver blocks on `done` 64
times; all 64 must complete in order. This exercises the new accounting across many
iterations (a miscount would deadlock or terminate early). The full existing
scheduler suite (linear/parallel/failure/cycle/gate/loop) passes, `-count=3`, as the
equivalence regression guard.

**Negative control:** making the driver consume two signals per iteration
(`<-done; <-done`) deadlocked the deep chain — the test hit its 15 s timeout with a
goroutine dump. Restored; test passes.

## Verification / gate
- `kernel/scheduler` tests pass (`-count=3`).
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
