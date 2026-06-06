# M526 — Mutation testing pulse: pin QuietHours.Active window edges

## Context
Continuing `kernel/pulse` into `briefing.go` — `QuietHours.Active`, the daily quiet-window
check (only alert/act briefs break through during it). Run with `GOMAXPROCS=3` (CPU-capped).

## The genuine gap (closed)
```
if q.Start < q.End { return h >= q.Start && h < q.End }      // normal window
return h >= q.Start || h < q.End                             // wraps midnight
```

The only prior coverage was a single wrap-window assertion (2am inside, noon outside) in
`TestParseQuietHours`. So the entire **normal** (`Start < End`) branch was untested, and
neither branch was checked at its exact hour edges. Four non-equivalent mutants survived
(confirmed by hand-applied negative control):
- normal `h >= Start → h > Start` and `h < End → h <= End`;
- wrap `h >= Start → h > Start` and `h < End → h <= End`.

Each shifts the window by an hour at an edge — quiet hours starting/ending one hour
early or late, which either lets a notify ping break through when it should be held, or
holds one when it should send. The window is inclusive of Start and exclusive of End.
(The `Start == End → false` guard was already killed by the existing wrap test.)

## Fix
Added `TestQuietHours_Active`: a table over disabled / normal (9–17) / wrap (22–7) /
degenerate (9–9), asserting the inclusive-start and exclusive-end edges of both branches
(`9`→in, `17`→out; `22`→in, `0`/`6`→in, `7`→out) plus the disabled and Start==End cases.

## Negative control (manual, CPU-capped)
All four edge mutants (normal + wrap `>= → >` and `< → <=`) FAIL under the new test.
Restored byte-for-byte (`git diff --ignore-all-space` on briefing.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-three packages (M490–M526)
pulse counted once; its salience bands (M523), novelty TTL (M524), disk thresholds (M525),
and now the quiet-hours window (M526) are all pinned at every inclusive edge.
