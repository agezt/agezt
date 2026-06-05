# M426 — state-namespace poison + Edict hard-deny false-positive

## Context
The final security-core review (Edict trust-ladder/policy engine, state projection
store, anomaly circuit breaker). **Edict's critical invariants were confirmed sound**
(no HIGH): the hard-deny floor is un-overridable by any trust level/ceiling, fires
before lookup/clamp/ask, and survives snapshot/overlay/replay; `DecideWithCeiling`
only ever tightens; unknown capabilities default-deny; the snapshot is trusted only
when its content hash matches the journaled `policy.compacted` hash. The anomaly
monitor recovers a panicking `onTrip` and its threshold math is correct. Two
non-HIGH bugs surfaced.

## Fixes

### MEDIUM — `state.Set` poisons a namespace on a bad RawMessage (`kernel/state/state.go`)
`Set` mutated the in-memory map (`bucket[key] = raw`) before `snapshotLocked`
persisted. `toRawMessage` passed a `json.RawMessage` through unvalidated, so an invalid
one (a malformed plugin/tool result via the documented passthrough path) was stored,
then the whole-namespace marshal in `snapshotLocked` failed. The error was returned but
the bad entry stayed in the map — so every subsequent `Set` to that namespace failed
(it re-marshals the whole bucket) and `Get` returned invalid JSON diverging from disk,
for the rest of the process. Fix: `toRawMessage` now `json.Valid`-checks a RawMessage
and returns an error before `Set` touches the map, keeping in-memory == disk.

### LOW — Edict hard-deny false-positive on word-split prose (`kernel/edict/edict.go`)
To catch the space-padded fork bomb (`:(){ :|:& };:`, whose internal spaces are
syntactically optional), `denyCandidates` derived a fully whitespace-stripped variant
and substring-matched it against every floor rule. Stripping *all* whitespace
collapsed ordinary prose onto alphabetic rules: `re boot the server` →
`reboottheserver` matched `reboot`; `mk fs` → `mkfs`; `power off` → `poweroff`;
`wipe fs` → `wipefs`. A hard-deny has no override, so a legitimate command was
permanently blocked (availability bug; fails closed, hence LOW). Fix: new
`stripPunctAdjacentWhitespace` removes a whitespace char only when it borders a
punctuation rune, so the fork bomb still normalises (its spaces sit next to `{ | & ;`)
but two alphanumeric words are never merged.

## Verification
- **`kernel/state/state_test.go`** `TestSet_InvalidRawMessageRejectedNoPoison`: an
  invalid `json.RawMessage` is rejected, a later `Set` to the same namespace succeeds,
  the poison key never lands, and the prior values stay valid.
  - **Negative control:** removing the `json.Valid` check → the later `Set` fails
    (namespace poisoned) → FAIL. Restored.
- **`kernel/edict/edict_forkbomb_test.go`** `TestHardDeny_WordSplitNotFalselyDenied`:
  `mk fs.go` / `re boot the server` / `power off` / `wipe fs` are NOT hard-denied; the
  existing fork-bomb spacing-variant and `rm -rf /` padding tests still pass.
  - **Negative control:** forcing full-strip → the word-split cases are falsely denied
    → FAIL (fork bomb still denied). Restored.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2284** passing (was 2282; +2). CHANGELOG
  Reliability entries.

## Review status
This closes the security-core review. Edict (hard-deny floor, ceiling clamp, default-
deny, snapshot integrity, toolmap fail-closed), state (per-op concurrency), and the
anomaly breaker (panic-safe `onTrip`, correct threshold/latch) were found sound. The
code-review-driven hardening arc M412–M426 has now swept every kernel subsystem.
