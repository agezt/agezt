# M345 — World-model Relate dedup + empty-name coverage

## Why
Priority-A coverage on the world-model graph (the agent's entity/belief store).
`Graph.Relate` creates a relation between two entities, auto-creating missing
endpoints, and is keyed by a deterministic `RelationID(from, verb, to)` so a
restated link reinforces rather than duplicates. The existing
`TestRelateResolvesEndpointsAndJournals` covered the create + auto-create-endpoint
path, but two genuine behaviours were untested:

1. **Dedup / reinforce on a duplicate relate.** Restating the same (from, verb, to)
   must yield ONE edge, not two — otherwise the graph accumulates duplicate
   relations every time the model mentions a known link, polluting `Neighbors` and
   resolution. The reinforce must also preserve `CreatedMS` and keep the weight at
   the clamp ceiling (1.0) rather than overflowing.
2. **`ErrEmptyName` guard.** Relating with an empty/blank endpoint name must error,
   not create a junk entity or a malformed edge.

## What
Test-only. Added to `kernel/worldmodel/manager_test.go`:
- **`TestRelateDeduplicatesAndReinforces`** — two identical `Relate` calls produce
  the same `RelationID`, `Relations()` returns exactly one edge, the reinforced
  weight stays `1.0` (clamped, not 1.1/2.0), `CreatedMS` is preserved, and exactly
  two `worldmodel.relation.upserted` events are journaled (create + reinforce).
- **`TestRelateRejectsEmptyName`** — an empty `from` name and a blank (whitespace)
  `to` name each return `ErrEmptyName`.

## Verification
- `go test ./kernel/worldmodel -run Relate -v` — all three relate tests pass
  (1 pre-existing + 2 new).
- `gofmt -l` clean; `go vet ./kernel/worldmodel/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2070** passing (was 2068; +2), `go test ./...` exit
  0. `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — dedup-by-RelationID and the empty-name guard already
  worked; this pins them. The rest of the world-model (Upsert create/reinforce/
  alias-merge, Forget/Revive, Resolve exact/alias/rank/tombstone-exclusion, Decay
  floor/idempotent, Neighbors, IsActiveSubject, normalization, FileStore persist)
  was already covered; this milestone targets only the uncovered Relate edges.
- Found (and noted) that `clampWeight` caps at 1.0 and relations are created at
  weight 1.0, so a reinforce is weight-neutral by design — the test asserts that
  invariant rather than a (non-existent) increase.
