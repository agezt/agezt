# M503 — Mutation testing worldmodel: pin first-writer-wins entity provenance

## Context
Fourteenth package in the mutation pass: `kernel/worldmodel` (the agent's structured
knowledge store — entities, relations, decay, resolve scoring). Run with
`GOMAXPROCS=3` (CPU-capped). Score 0.429 over 445 mutants. The package is dominated by
float scoring/decay-threshold mutants, most of which are equivalent (e.g. the
decay clamp `newWeight < floor` then `= floor` is a no-op at the boundary) or would
require brittle exact-float setups; the clean, high-value LOGIC gap was entity
provenance.

## The genuine gap (closed)
`Graph.Upsert` attributes an entity's source event first-writer-wins:
`if ev != nil && e.SourceEvent == "" { e.SourceEvent = ev.ID }` — set once, on first
creation; preserved on every later re-observation. SourceEvent is the provenance used
for audit/causation ("why does the world model know about X?"), and the meaningful
answer is the *origin* event, not the latest mention. `TestUpsertCreatesAndJournals`
only checks that a *created* entity carries provenance, not that re-observation
*preserves* it — so the mutation `&&`→`||` **survived**: it overwrites SourceEvent on
every re-observation (last-writer), losing the origin.

## Fix
`kernel/worldmodel/provenance_test.go` (internal `package worldmodel`):
`TestUpsert_PreservesOriginalProvenanceOnReObserve` — create "Lictor" under corr-1
(records SourceEvent ev1), re-observe the same Kind+Name under corr-2, assert
`created == false` and `SourceEvent` is still ev1.

## Negative control (manual, CPU-capped)
Applying the survivor (`&& → ||`) makes the test fail — `re-observe overwrote
provenance: got "01KTEMQ3BFP…" want the original "01KTEMQ3BDJ…"`; restored byte-for-byte
(`git diff --ignore-all-space` on manager.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — fourteen packages (M490–M503)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel — plus the controlplane primary-token auth gate
verified solid. Note: worldmodel's score is the lowest tier because its surviving
mutants are predominantly float scoring/decay thresholds (equivalent or brittle to
pin) rather than the clean integer/logic gaps that dominated the other packages — the
one clean logic gap (provenance preservation) is now closed.
