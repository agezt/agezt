# M199 — Crash-safe one-shot schedules (at-least-once)

## Why
The cadence engine fired due schedules in two coupled steps inside `Store.Due`:
it advanced (or, for a one-shot, **removed**) the entry and persisted, then the
engine launched the run on a goroutine. For a one-shot (`ModeOnce`) this opened a
silent-drop window:

1. `Due(now)` sees the one-shot is due → removes it from the store → `save()`.
2. The engine launches the run.
3. **If the daemon crashes between (1) and the run completing**, the one-shot is
   already gone from `schedules.json`. It never ran, and a restart cannot recover
   it — the firing is lost with no trace.

This is the cadence-review **H2** follow-up. The eager remove/advance is the right
*at-most-once* choice for recurring schedules (a missed interval/daily slot
self-corrects at the next slot, and re-running a stale recurring slot after a
restart is more disruptive than skipping it). But for a one-shot — an intent the
operator scheduled to happen *once* at a specific time — silently dropping it on a
crash is the wrong trade. A one-shot should be **at-least-once**: survive a crash
and re-fire on restart.

## What
The removal of a one-shot is deferred from "when it becomes due" to "after its run
completes", so the entry persists across the entire run (including a crash window).

- **`Store.Due`** no longer removes or advances a `ModeOnce` entry. It returns the
  one-shot as due but leaves it in the store, enabled and due. Recurring
  (interval/daily/window) entries still advance eagerly and persist in `Due`,
  exactly as before — their at-most-once semantics are unchanged.
- **`Store.CompleteFiring(id) (bool, error)`** — new. Removes a `ModeOnce` entry and
  persists; a no-op (returns false) for recurring entries and unknown ids. This is
  the single place a one-shot is retired, and only a completed run reaches it.
- **`Engine.fireDue`** — the per-run goroutine calls `store.CompleteFiring(id)` after
  `e.run(...)` returns (whether it succeeded or errored — so a permanently-failing
  one-shot is retired rather than retry-storming every tick). It runs *before* the
  deferred `running.Delete`, so no tick can re-fire the one-shot in the gap between
  its removal and clearing the in-flight guard.

Result:
- **Crash mid-run** → the one-shot is still in `schedules.json`, enabled and due →
  it re-fires on restart (at-least-once).
- **Normal completion** → retired exactly once; never fires again.
- **Concurrent ticks while the run is in flight** → the engine's `running` map
  (LoadOrStore busy check) skips the duplicate, so the single run is not doubled,
  even though the entry remains due in the store.

## Tests (+3; one existing test updated for the new contract)
- `kernel/cadence/once_crashsafe_test.go`
  - `TestStore_CompleteFiring_RecurringIsNoOp` — `CompleteFiring` removes only
    one-shots; a recurring entry and an unknown id are no-ops.
  - `TestEngine_Once_RemovedAfterRun` — happy path: a one-shot fires, its run
    completes, it is then removed, and a later tick does not re-fire it.
  - `TestEngine_Once_SurvivesCrashWindow` — the core guarantee: while the run is in
    flight (blocked via the recorder's `block` channel, standing in for the crash
    window) the entry is still in the store (Count==1), and a second tick does not
    start a second run; once the run completes the entry is removed (Count==0) and
    the run count is exactly 1.
- `kernel/cadence/cadence_test.go` — `TestStore_Once_FiresOnceAndSelfRemoves`
  renamed to `TestStore_Once_FiresAndCompletes` and updated: `Due` now leaves the
  one-shot in place (Count stays 1 and it stays due across repeated `Due` calls);
  `CompleteFiring` is what removes it. This locks in the new Store-level contract.

## Verification
- `go test ./...` — 1620 passing (1617 + 3 new), 0 failing.
- `go vet ./kernel/cadence/` — clean.
- `gofmt -l` (CRLF-normalized) clean on all touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/cadence/cadence.go` — `Due` defers one-shot removal; new `CompleteFiring`;
  `fireDue` completes the firing after the run.
- `kernel/cadence/once_crashsafe_test.go` — new crash-safety tests.
- `kernel/cadence/cadence_test.go` — updated one-shot Store-contract test.

## Scope note
This closes cadence-review **H2** for one-shots. Recurring schedules retain
at-most-once advance deliberately; that asymmetry is now explicit in the `Due`
doc comment. The TOCTOU between the in-flight guard and a restart is intentionally
resolved toward at-least-once for one-shots (re-fire on crash) and at-most-once for
recurring (skip the missed slot).
