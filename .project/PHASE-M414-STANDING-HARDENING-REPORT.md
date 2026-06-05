# M414 — Standing-order hardening (two LOW review findings)

## Context
The remaining two LOW-severity findings from the Chronos code-review pass (after the
M413 HIGH fix). Both are real but minor; bundled into one milestone.

## Fixes

### Unbounded growth of the per-order bookkeeping maps
`kernel/standing/runner.go` (`lastFireMS`) and `kernel/standing/cron.go`
(`lastFired`) key a last-fire timestamp by `o.ID` to enforce the cooldown / once-
per-minute dedup. Entries were only ever **added**; when an order is removed via
`Store.Remove`, its entry stayed forever. Order ids are unique per creation, so a
long-lived daemon with add/remove churn leaked memory bounded only by the total
number of distinct ids ever created. Fix: new shared helper `pruneToLive(map,
orders)` drops entries whose id is no longer in the live order set; the runner and
cron loop both call it each pass. Cheap — it only rebuilds the live-id set when the
map has actually outgrown the order list (common no-churn case is a single length
comparison), and it runs single-goroutine so no locking is needed.

### `usdToMicrocents` overflow / non-finite
`cmd/agt/budget.go` (reached by `agt standing add --budget` and the other budget
flags). `int64(d*1e9 + 0.5)` is **undefined** in Go when the float overflows int64,
and `NaN`/`+Inf` slipped past the existing `d < 0` check. A `--budget 99999999999`
produced `-9223372036854775808` (a negative spend cap) — silently mis-configuring
the guard. Fix: reject non-finite (`math.IsNaN`/`math.IsInf`) and any amount whose
microcents value `>= math.MaxInt64`, with clear errors, before converting.

## Verification
- **`kernel/standing/standing_test.go`** `TestPruneToLive`: a stale id is dropped,
  live entries kept intact, and a no-stale map is left untouched.
  - **Negative control:** making `pruneToLive` a no-op → the stale `"gone"` entry
    survives → test FAILs. Restored byte-identical.
- **`cmd/agt/budget_test.go`** `TestUsdToMicrocents` (extended): `99999999999`,
  `1e30`, `Inf`, `NaN` all rejected; the existing valid/negative/empty cases still
  pass.
  - **Negative control:** gutting the overflow guard (`>= math.MaxFloat64`, never
    true) → the two overflow cases return `-9223372036854775808` instead of erroring
    → test FAILs (confirming the exact garbage-negative-cap bug). Restored
    byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2262** passing (was 2261; +1). CHANGELOG
  Reliability entries added.

## Status
This closes every finding from the two-pass code review of the Chronos standing-
order feature (M412 store rollback + local-clock cooldown; M413 panic containment;
M414 map growth + budget overflow). SPEC-16 §4 Chronos is complete and hardened for
everything buildable + verifiable offline.
