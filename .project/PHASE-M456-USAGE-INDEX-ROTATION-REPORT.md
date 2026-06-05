# M456 — Governor usage index: two-generation rotation (no partial-sum under-count)

## Context
The governor keeps a bounded, best-effort, in-memory per-correlation token index
(`usage map[string]usageTokens`) behind `UsageFor`, the O(1) fast path for the
OpenAI-compat `usage` REPORTING field (avoiding an O(journal) scan per API
response). It is explicitly NOT used for billing or ceiling enforcement — those go
through `spentToday` and the authoritative journal. The contract for the fast path
is: serve the COMPLETE summed usage for a correlation, or MISS (`ok=false`) so the
caller falls back to the journal scan. A *partial* sum served with `ok=true` breaks
that contract — it's a silent under-report, never corrected by the fallback.

## The bug
`indexUsageTokens` bounded memory by dropping the whole map when it reached
`usageIndexCap`:

```go
if g.usage == nil || len(g.usage) >= usageIndexCap {
    g.usage = make(map[string]usageTokens, 64) // wholesale drop
}
e := g.usage[corr]; e.in += in; e.out += out; g.usage[corr] = e
```

If the cap was hit while a multi-call run was still in flight, that run's
accumulated entry was wiped. Its *subsequent* calls (same correlation) then created
a fresh zero-based entry holding only the post-drop tokens. A later
`UsageFor(corr)` returned that **partial** sum with `ok=true` — so the caller
trusted it and did NOT fall back to the journal, under-reporting tokens on the API
`usage` field. The old comment ("no wrong sum is ever served") was incorrect: a
clean miss would have been correct; a partial hit is not.

## The fix
Two-generation rotation (live + previous), memory bounded at 2×cap:

- Writes land in the live map.
- A write for a correlation already present **only** in the previous generation
  *migrates* that accumulated entry into the live map before adding — so the live
  entry always holds the complete running sum.
- When the live map fills (`>= usageIndexCap`), it rotates to become the previous
  generation and a fresh live map starts.
- `UsageFor` checks live then previous; migrate-on-write guarantees a correlation
  is never split across both, so the first hit is the complete sum.
- A correlation is dropped only when it ages out of BOTH generations untouched —
  then `UsageFor` cleanly misses and the caller falls back to the journal.

No behaviour change for billing or ceilings (always journal-authoritative).

## Test + negative control
`kernel/governor/usage_index_internal_test.go`:
`TestUsageIndex_NoPartialSumAcrossRotation` — records a correlation, fills the live
generation to force a rotation (pushing it to the previous generation), records the
same correlation again, and asserts `UsageFor` returns the COMPLETE sum
`(105,43,true)` — plus that a correlation living only in the previous generation is
still a hit. Existing `TestUsageIndex_AccumulatesAndReports` and
`TestUsageIndex_Bounded` still pass (the live generation still never exceeds cap).

**Negative control:** disabling the migrate-on-write consolidation
(`hadPrev && false`) made the test report `UsageFor(live) = (5,3,true)` — exactly
the partial under-count — `--- FAIL`. Restored; test passes.

## Verification / gate
- `kernel/governor` tests pass (all `TestUsageIndex*` + suite).
- gofmt-clean on staged LF blobs (`git show :file | gofmt -l` empty), `go vet`
  clean, `GOOS=linux` build clean, full `go test ./...` exit 0, `go.mod`/`go.sum`
  unchanged.
