# M331 — Integration test for the live-HITL approval routing

## Why
The runtime's `policyHook` (kernel/runtime/runtime.go:693) is the security-critical
glue that turns an Edict `RequiresApproval` verdict into a live human-in-the-loop
gate: it submits to the `approval.Registry`, blocks the tool-loop until an operator
decides, and maps grant→allow / deny→block. The `approval.Registry` itself is
well-tested (Submit/Resolve/grant/deny in `approval_test.go`), but the **runtime
glue had no test** — nothing proved that an Ask-class tool call actually routes
through the registry and that the decision is honoured end to end. For a path that
decides whether a tool runs, that's a coverage gap worth closing (it was the
documented follow-up from the M330/edict doc work).

## What
- **`kernel/runtime/approval_test.go`** (new, test-only — no production change):
  a side-effect-free `probeTool` whose name maps (via the default toolmap rule) to
  the `"approvalprobe"` capability, pinned to `LevelAsk` with `AskPolicy=AskPrompt`
  so a call requires approval. Two black-box integration tests drive the real
  `runtime.Kernel` via `k.Run` with a mock provider that emits a tool call then a
  final answer, resolving the pending approval from a second goroutine:
  - **Granted**: the run parks on the approval, the pending request carries the
    right tool + capability, and once granted the run resumes and **the tool
    actually executes** (invocation counter = 1), finishing with the answer.
  - **Denied**: the operator denies, **the tool never executes** (counter = 0),
    and the run still completes (the loop feeds the denial back and the model
    produces its final answer).

  Uses a no-op probe tool rather than shell/file so there are zero side effects on
  either branch.

## Verification
- Both tests pass; **15× stress** run clean (they coordinate two goroutines via a
  poll-until-pending helper with a 3 s deadline — no flakiness observed).
- Full suite **2025** passing, `go test ./...` exit 0; `gofmt -l` clean; `go vet`
  clean; `GOOS=linux` build clean; `go.mod` / `go.sum` unchanged. No CHANGELOG
  entry — test-only, no user-facing behaviour change.

## Scope notes
- Confirms the behaviour the M330/edict-doc fix described: AskPrompt → live
  approval routing is fully wired and now regression-protected.
- The tests exercise the default toolmap rule (`CapabilityForToolCall` →
  `Capability(toolName)` for an unknown tool name), so they also lock in that
  unknown tools are gated by a capability named after the tool — not silently
  allowed.
