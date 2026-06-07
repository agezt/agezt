# M505 — Mutation testing memory: pin first-writer-wins record provenance

## Context
Sixteenth package in the mutation pass: `kernel/memory` (the agent's persistent
memory store — paired with worldmodel; content-keyed records, reinforce, decay,
recall). Run with `GOMAXPROCS=3` (CPU-capped). Score 0.432 — like worldmodel, the
package is float scoring/decay-heavy, so most survivors are equivalent or brittle. The
clean logic gap is record provenance — the same first-writer-wins pattern fixed in
worldmodel (M503).

## The genuine gap (closed)
`Manager.Remember` on a reinforce (re-remembering an existing record) copies the
original source event — `rec.SourceEvent = existing.SourceEvent` — and only sets it
from the current event when still empty — `if ev != nil && rec.SourceEvent == ""`. The
origin event is the meaningful provenance for audit/causation.
`TestRememberCreatesAndJournals` only checks that a *created* record carries provenance,
not that a *reinforce* preserves it, so **two** mutants survived: removing the
`existing.SourceEvent` copy, and flipping the guard `&&`→`||` — either overwrites the
provenance with the latest mention (last-writer), losing the origin.

## Fix
`kernel/memory/provenance_test.go` (internal `package memory`):
`TestRemember_PreservesProvenanceOnReinforce` — remember a fact under corr-1 (records
SourceEvent ev1), re-remember the identical fact under corr-2, assert `created == false`
and `SourceEvent` is still ev1.

## Negative control (manual, CPU-capped)
Both survivors fail the test: removing `rec.SourceEvent = existing.SourceEvent` → FAIL;
flipping `&& → ||` → FAIL. Restored byte-for-byte (`git diff --ignore-all-space` on
manager.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — sixteen packages (M490–M505)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory — plus the controlplane
primary-token auth gate verified solid. The two knowledge-store packages (worldmodel,
memory) shared the same first-writer-wins provenance gap, now closed in both; their
remaining survivors are float scoring/decay thresholds (equivalent or brittle to pin).
