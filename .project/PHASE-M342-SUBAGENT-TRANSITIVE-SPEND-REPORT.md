# M342 — Sub-agent spend cap: transitive-descendant lock-in

## Why
Priority-A coverage on a security/correctness-critical resource bound. The M48
sub-agent **spend cap** (`SubAgentMaxSpendMicrocents`) is one of three guards that
keep delegated multi-agent runs from running away (alongside depth and fan-out).
`runtime.subAgentSpendMicrocents` implements it as a BFS over the journal's
`subagent.spawned` links, summing spend across the **full transitive descendant
tree** (children, grandchildren, …) of the spawning run, with a `seen` set
guarding malformed cycles.

But the existing `TestSubAgent_SpendGuard` opens its kernel with
`SubAgentMaxDepth: 1` — so a sub-agent can never itself delegate, and there are
never any grandchildren. The whole point of `subAgentSpendMicrocents` (transitive
summation + BFS + cycle guard) was therefore exercised only at the trivial
direct-children depth. A regression that summed only direct children would have
kept the suite green while letting an operator **evade the spend cap by nesting
delegation one level deeper** — a real cost-control hole.

## What
Test-only. Added to `kernel/runtime/subagent_test.go`:
- **`openSpendKernelDepth`** — `openSpendKernel` with a caller-chosen max depth
  (the existing helper hardcodes depth 1), so a child can delegate to a grandchild.
- **`TestSubAgent_SpendGuardCountsTransitiveDescendants`** — a depth-2 run
  (lead → child → grandchild). Each LLM call costs $0.0021; by the lead's 2nd
  delegate the transitive descendant spend is child ($0.0042) + grandchild
  ($0.0021) = $0.0063. With a $0.0050 cap that 2nd delegation is **refused** —
  and crucially, a buggy direct-children-only sum ($0.0042 < $0.0050) would have
  **admitted** it. The spawn count (2, not 3) distinguishes correct transitive
  accounting from the direct-only bug; depth (well under the limit) and fan-out
  (unbounded) are ruled out as the cause, so the refusal can only come from
  counting the grandchild.

## Verification
- `go test ./kernel/runtime -run SubAgent_SpendGuardCountsTransitive -v` — passes;
  exactly 2 `subagent.spawned` events, lead completes with "lead done".
- `gofmt -l` clean; `go vet ./kernel/runtime/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2065** passing (was 2064; +1), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — `subAgentSpendMicrocents` already walked the transitive
  tree correctly; this pins that contract so the BFS can't be silently reduced to
  direct children. The other two bounds (depth, fan-out) and the spend cap's
  direct-child and unbounded-default cases were already covered; the spend cap's
  transitive property is now covered too.
- The `seen` cycle guard remains defence-in-depth against a malformed link;
  exercising it would require fabricating a cyclic `subagent.spawned` payload
  directly in the journal (the runtime never produces one), which is a deeper
  white-box effort left as a noted follow-up rather than guessed at here.
