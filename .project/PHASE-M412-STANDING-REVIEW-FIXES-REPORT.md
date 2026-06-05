# M412 — Standing-order store correctness fixes (code-review pass)

## Context
Per the standing goal "ne yanlıs" (find what's wrong), a code-review pass over the
Chronos standing-order package (M403–M411) found two genuine correctness defects.
Both are internal reliability bugs — no API or CLI shape change — so this milestone
is a Fixed/Reliability entry, not a new feature.

## Bugs found & fixed

### BUG 4 — cooldown keyed off the event timestamp (`kernel/standing/runner.go`)
The per-order event-trigger cooldown compared `ev.TSUnixMS - lastFire` against the
window. An externally-sourced event (webhook/mesh) can carry an attacker- or
clock-skew-controlled timestamp: a far-future stamp would store a far-future
`lastFire` and **permanently suppress** the order; a far-past stamp would release
it prematurely. Fix: added `RunnerConfig.Now func() time.Time` (nil → `time.Now`,
injectable for tests) and switched the cooldown to the runner's **local** clock,
which no untrusted event can influence. The doc comment was also softened: a run's
downstream events can still re-match a broadly-subscribed order, but only after the
cooldown elapses (rate-bounded, not impossible).

### BUG 5 — no rollback on a failed durable write (`kernel/standing/standing.go`)
`SetEnabled` and `Remove` mutated the in-memory order/slice **before** `save()`.
If the atomic write failed (full disk, permissions), the function returned the
error but the in-memory view kept the mutation — so the running daemon showed a
pause/removal that never reached disk and would silently vanish on the next reopen
(live view diverges from durable state). Fix: both capture the prior state and
restore it in the `save()` error branch (mirrors the existing rollback in `Add`).

## Verification
- **`kernel/standing/runner_test.go`** `TestRunner_CooldownUsesInjectedClock`:
  injects an `atomic.Int64` clock; first event fires, a second event with the clock
  unchanged is held by the cooldown, advancing the clock +2 min lets it fire again.
  - **Negative control:** reverting the cooldown to key off `ev.TSUnixMS` → the
    test FAILs ("should fire again, got 1"); restored byte-identical.
- **`kernel/standing/standing_test.go`** `TestStore_RollsBackOnSaveFailure`: forces
  `save()` to fail by turning the atomic-write temp path into a directory, then
  asserts `SetEnabled`/`Remove` both error AND leave the live view unchanged
  (`Enabled` true, order present, `Count`==1).
  - **Negative controls:** neutering the `SetEnabled` rollback → FAIL at the Enabled
    assertion; neutering the `Remove` rollback → FAIL at the order-present + Count
    assertions. Both restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2260** passing (was 2258; +2). CHANGELOG
  Reliability entries added.

## Scope notes
SPEC-16 §4 Chronos remains functionally complete for everything buildable +
verifiable offline (M403–M411). This milestone only hardens the store's durability
and the runner's cooldown against transient write failures and untrusted event
timestamps. The deliberately-unbuilt `observers`/salience pipeline (references a
named-observer registry the DSL doesn't define) is unchanged.
