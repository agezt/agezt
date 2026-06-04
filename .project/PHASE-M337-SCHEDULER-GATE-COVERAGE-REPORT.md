# M337 — Scheduler GateNode fail-closed + LoopNode guard coverage

## Why
Priority-A work under the v1.0-conformance goal: test coverage on a
security/correctness-critical path. The scheduler's `GateNode` (SPEC-06 plan
gate) pauses a plan and routes a request to `approval.Registry`; its doc comment
states "deny / **timeout** / cancel cause the node to fail and the plan to
abort." But `scheduler_test.go` only exercised the **grant** and **deny** paths.
The two remaining terminal outcomes — **timeout** (nobody answers) and **cancel**
(ctx torn down) — are exactly the fail-closed cases that matter most: if a plan
gate that nobody answers were to silently release its guarded branch, the gate
would be worthless. That fail-closed behaviour had no lock-in test. The node
error guards (`GateNode.Approvals==nil`, `LoopNode.Runner==nil`, empty intent)
were also untested.

This is a genuine gap (verified by reading `nodes.go` against `scheduler_test.go`),
fully offline-verifiable, and on a correctness-critical path — no production code
change, just locking in behaviour that already exists so it can't regress.

## What
Test-only. Added to **`kernel/scheduler/scheduler_test.go`**:
- **`TestGateNode_TimeoutAbortsDownstream`** — a gate with a 50ms approval
  timeout that nobody resolves → `DecisionTimeout` → node fails → plan aborts.
  Asserts the guarded `execute` node never runs, the gate is in `res.Errors`, and
  the journal records exactly one `approval.timeout` + one `plan.failed`
  (fail-closed, SPEC-06 §3.4 "Time-outs default to deny").
- **`TestGateNode_CancelAbortsDownstream`** — cancels the plan ctx once the gate
  is parked in the approval queue → `DecisionCancel` → node fails → guarded branch
  never runs.
- **`TestGateNode_NilApprovalsErrors`** — a gate wired without an `approval.Registry`
  fails the plan rather than silently passing (which would defeat the gate).
- **`TestLoopNode_GuardsRejectBadConfig`** (2 subtests) — a `LoopNode` with no
  `Runner`, and one with an empty intent, each fail the node instead of panicking
  or running an empty agent loop.

## Verification
- New tests pass (`go test ./kernel/scheduler -run 'GateNode|LoopNode' -v`); the
  timeout test genuinely waits out the 50ms window and observes the synthesised
  deny.
- `gofmt -l` clean; `go vet ./kernel/scheduler/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2050** passing (was 2044; +6 `PASS:` lines = 4 new test
  functions + 2 subtests), `go test ./...` exit 0. `go.mod` / `go.sum` unchanged.

## Scope notes
- No production behaviour change — the timeout/cancel/guard paths already worked
  (`approval.Submit` synthesises `DecisionTimeout`/`DecisionCancel`; `GateNode.Run`
  returns an error for any non-grant outcome). This milestone makes that contract
  regression-proof.
- The grant + deny paths were already covered; the gate's four terminal outcomes
  (grant/deny/timeout/cancel) are now all exercised end-to-end through the real
  scheduler executor + real approval registry + real journal.
