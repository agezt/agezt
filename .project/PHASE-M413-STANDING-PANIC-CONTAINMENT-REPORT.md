# M413 — Standing-order panic containment (HIGH reliability fix)

## Context
A second code-review pass over the Chronos feature (the M412 follow-up, continuing
"ne yanlıs") found a HIGH-severity reliability bug: a panic in a fired standing
order crashed the entire daemon, directly contradicting the package's documented
no-crash guarantee.

## The bug
`StartRunner` (kernel/standing/runner.go) and `StartCron`/`tickCron`
(kernel/standing/cron.go) dispatch each matched order with a bare
`go fire(ctx, ord, subject)`. Their `defer func(){ _ = recover() }()` sits on the
**loop** goroutine — not on the child goroutine that actually runs `fire`. The
daemon's `fire` closure (cmd/agezt/main.go `buildStandingRunner`) calls:
- `k.RunWith(...)` — a full governed agent run (provider calls, tool execution,
  plugins) — any of which can panic;
- `k.Bus().Publish(...)`;
- `brief(...)` → `standingBrief` → `channelSend` (network I/O to webhooks/channels).

A panic on a goroutine with no recovering deferred frame terminates the **process**,
killing every in-flight run and the control plane. An input that panics a tool,
reached via a broadly-subscribed standing order, was a remote daemon-kill. The
package doc ("a panic in the loop is recovered so a runner bug never crashes the
daemon", runner.go / cron.go) was simply not true for the dispatched run.

## The fix
Two layers (defense in depth):
1. **`kernel/standing` — universal backstop.** New unexported `safeFire(fire)`
   wraps a `FireFunc` in `defer recover()`. `StartRunner` and `StartCron` wrap their
   `fire` once (`fire = safeFire(fire)`) before the loop, so EVERY dispatched order —
   for any FireFunc a caller passes — is contained. This makes the documented
   guarantee true at the package boundary.
2. **`cmd/agezt/main.go` — diagnostic recover.** The daemon's `fire` closure now
   defers a recover that journals a new `standing.error` event (id/name/trigger/
   panic message) so a crashed order stays diagnosable via `agt journal`, rather than
   vanishing into a silent recover. (This runs first; the package backstop is the
   catch-all for any path that doesn't journal.)
3. **`kernel/event/kinds.go`** — new `KindStandingError = "standing.error"`
   (mirrors `KindChannelError`), registered in `knownKinds`.

## Verification
- **`kernel/standing/standing_test.go`** `TestSafeFire_ContainsPanic`: a FireFunc
  that panics, called synchronously through `safeFire`, returns normally (panic
  contained) and the wrapped fire still ran; a normal fire passes through.
  - **Negative control:** removing the `recover()` from `safeFire` → the synchronous
    call panics the test goroutine → the test FAILs with the panic stack. Restored
    byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2261** passing (was 2260; +1). CHANGELOG
  Reliability entry added.

## Why not test the main.go closure directly
The journaling recover lives in a local closure inside `buildStandingRunner` over a
real `*Kernel`; constructing a Kernel whose `RunWith` panics is heavy and brittle.
The package-level `safeFire` test proves the containment mechanism, and the
`standing.error` kind registration is covered by the event package. The two layers
together guarantee: no crash (safeFire, tested) + diagnosable (standing.error,
journaled by the daemon closure).

## Other review findings (deferred to M414)
The same review flagged two LOW issues — unbounded growth of the per-order cooldown
maps (`lastFireMS`/`lastFired`) when orders are removed, and an `int64` overflow in
`usdToMicrocents` for absurd `--budget` values. Both are real but minor; bundled
into the next milestone.
