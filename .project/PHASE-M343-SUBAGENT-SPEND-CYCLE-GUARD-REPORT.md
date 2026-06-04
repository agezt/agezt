# M343 — Sub-agent spend BFS cycle-guard lock-in

## Why
The immediate follow-up flagged in M342. `runtime.subAgentSpendMicrocents` totals
sub-agent spend by walking the parent→child delegation graph (a BFS over
`subagent.spawned` links read straight from the journal), guarded by a `seen` set
"against a malformed cyclic link". The runtime never *emits* a cycle, but the BFS
consumes journal payloads as data — a corrupt, truncated-and-rewritten, or forged
journal could present `A→B` and `B→A`. Without the `seen` guard that BFS loops
forever, wedging the spend check (and therefore every subsequent delegation) of a
running daemon. The guard existed but had no test, so a refactor could silently
drop it.

## What
Test-only, **white-box** (`package runtime`, new file
`kernel/runtime/subagent_cycle_internal_test.go`) — the method is unexported and
the cycle cannot arise through the public `delegate` path, so it must be injected
directly:
- **`TestSubAgentSpendMicrocents_CycleGuardTerminates`** opens a real kernel,
  publishes a fabricated cyclic spawn graph to its bus/journal (`A` claims child
  `B`; `B` claims child `A`; each with a `budget.consumed` spend), then calls
  `k.subAgentSpendMicrocents("A")` on a goroutine behind a 2-second deadline.
  - It asserts the call **terminates** (a regressed/removed `seen` guard would
    spin `A→B→A→B…` forever and trip the timeout → `t.Fatal`).
  - It asserts the returned total is `2000` — B's spend only: A is excluded (the
    cap bounds descendants, not the spawner itself) and the cycle back to A is
    visited exactly once.

## Verification
- `go test ./kernel/runtime -run CycleGuardTerminates -v` — passes, returns
  immediately (well under the 2s guard deadline).
- `gofmt -l` clean; `go vet ./kernel/runtime/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2066** passing (was 2065; +1), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — the `seen` guard already terminated the BFS; this pins
  that contract so it can't be removed unnoticed.
- Together with M342 (transitive descendant summation) the spend cap's graph walk
  is now covered on both axes: depth (grandchild spend counts) and robustness
  (a cyclic link can't hang it).
