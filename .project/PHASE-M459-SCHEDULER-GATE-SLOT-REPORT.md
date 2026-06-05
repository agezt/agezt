# M459 — Scheduler: gate nodes don't consume a compute worker slot

## Context
The plan scheduler (kernel/scheduler/scheduler.go) bounds parallelism with a
semaphore `sem := make(chan struct{}, maxParallel)` — a worker pool that limits how
many nodes run concurrently. Node kinds include `KindLoop` (a compute/agent
tool-loop) and `KindGate` (a `GateNode` that calls `Approvals.Submit` and **blocks
until a human decides** or the timeout fires).

## The bug
The driver acquired a slot for every node, in the loop, before launching it:

```go
for _, id := range ready {
    wg.Add(1)
    sem <- struct{}{}   // blocks here if the pool is full
    go runNode(id)      // runNode releases the slot on return
}
```

Two problems:
1. **A gate awaiting a human held a compute slot for the entire approval window.**
   The slot is meant to bound *compute* parallelism, but a gate consumed one while
   merely *waiting on a person*. With `MaxParallel` concurrently-waiting gates (or a
   single gate at `MaxParallel: 1`), no other ready node could start until a human
   responded — an operator-visible stall of independent branches.
2. **Acquiring in the driver also serialized launches:** a gate listed after a
   slot-bound compute node couldn't even start until a compute slot freed.

## The fix
- Acquire the semaphore **inside** `runNode`, not in the driver — so launching a
  node never blocks the driver.
- **Gate nodes skip the semaphore entirely** (`holdsSlot := byID[id].Kind() !=
  KindGate`). A gate blocks only on its human approval, never on or holding a
  compute slot.

Compute parallelism is still bounded by `maxParallel` (compute goroutines park on
`sem <-` inside `runNode`). Termination logic is unaffected: it keys off
`started`/`completed` (set in `pickReady`/on completion), independent of the
semaphore. Gates being cheap blocked goroutines, not counting them is correct.

## Test + negative control
`kernel/scheduler/scheduler_test.go`: `TestGate_DoesNotConsumeComputeSlot` — a
`blockerNode` (compute) holds the only slot (`MaxParallel: 1`) and signals on entry;
the test then asserts the gate still reaches the approval queue (`PendingCount()==1`)
**while** the slot is held. Deterministic regardless of goroutine scheduling because
only the blocker ever needs the slot. Teardown releases the blocker and cancels the
run (which unblocks the gate's approval wait). All existing gate tests
(`TestGateNode_Granted/Denied/Timeout/Cancel/NilApprovals`) still pass.

**Negative control:** forcing gates to also take a slot
(`byID[id].Kind() != KindGate || true`) made the test FAIL — the gate seized the
single slot and the blocker never ran (`blocker compute node never ran`). Restored;
test passes.

## Follow-up noted (NOT changed)
The driver still polls `time.Sleep(time.Millisecond)` between readiness re-checks
(a correct-but-busy 1 ms wait, ~1 ms scheduling-latency floor). It is a perf item,
not a correctness defect; a completion-channel rewrite carries deadlock risk and is
deferred rather than churned in here.

## Verification / gate
- `kernel/scheduler` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
