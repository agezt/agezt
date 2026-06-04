# M367 — Anomaly auto-halt circuit breaker (SPEC-06 §5)

## SPEC audit (read-vs-code)
SPEC-06 §5 (autonomous-operation safety) and §9 (secure defaults) require:

> **Anomaly auto-halt:** detectors watch tool-call rate, spend rate, error rate,
> and repetition. A spike auto-engages `agt halt` and notifies the user.
> (Watches Pulse's own rate too.) … **Anomaly auto-halt on by default.**

**Verified gap (not assumed):** grepped the whole tree for
`anomaly|auto-halt|watchdog|supervisor|spike|runaway|circuit`. Found the
adjacent guards but NOT this one:
- Loop guard (M116) — suppresses *identical* tool calls *within one run*.
- Governor throttle (M106) — refuses calls over a per-minute cap, doesn't halt.
- Budget ceilings — stop on spend, not on rate.
- `agt halt` / `HaltWith` — manual only; nothing auto-engages it.

So a runaway/looping agent (or a Pulse storm) flooding tool calls across runs
had **no automatic circuit breaker** — a genuine SPEC-06 §5/§9 gap and a
security-critical one (priority A).

## What
New first-party subsystem, offline-verifiable, default-on.
- **`kernel/anomaly`** — `Detector` (pure sliding-window rate trip; disabled when
  ceiling/window ≤ 0) + `Start(ctx, bus, cfg, onTrip)` which subscribes to the
  bus, feeds `tool.invoked` into the detector, and on a trip publishes a
  `system.anomaly` event then calls `onTrip` exactly once (latched). The watcher
  goroutine recovers from panics (panic-containment invariant).
- **`kernel/event`** — new `KindAnomalyDetected = "system.anomaly"` (+ knownKinds).
- **`cmd/agezt/main.go`** — `buildAnomaly` wires `onTrip → k.HaltWith(reason)`;
  on by default (ceiling 120 tool calls / 10s window — ~12/s sustained, only a
  tight loop hits it); `AGEZT_ANOMALY_MAX_TOOLCALLS` (0 disables) +
  `AGEZT_ANOMALY_WINDOW` tune it; boot banner shows the setting.
- **`kernel/controlplane/config.go`** — both env vars added to the inventory
  (the completeness test enforces this).

v1 covers the tool-call-rate signal — the clearest runaway indicator and a
global cross-run/channel/Pulse breaker. The spend-rate / error-rate / repetition
signals §5 also names plug into the same `Detector` shape (noted follow-ups).

## Verification
- **Unit** (`detector_test.go`, 3): disabled-never-trips; trips only when the
  count *exceeds* the ceiling (5 ok, 6 trips); window-slide pruning (a steady
  2/s stream never trips, a 4-in-1s burst does).
- **Integration** (`monitor_test.go`, 3, real bus + journal): a 6-call spike
  fires `onTrip` AND journals a `system.anomaly` event; a disabled config starts
  no watcher and never trips under 50 calls; exactly-at-ceiling does not trip.
- **Live daemon demo** (mock provider, ceiling 1): a run making 2 tool calls
  tripped mid-run → banner `⚠ anomaly auto-halt engaged: tool-call rate
  anomaly: 2 tool calls within 2m0s exceeds ceiling 1`; `system.anomaly` (seq 14)
  + `kernel.halt` (seq 15) journaled; the run was cancelled; the next run was
  refused with `kernel is halted`. End to end on the real CLI/daemon path.
- `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2118** passing (was 2112; +6), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. CHANGELOG (Added, user-visible default-on guard).

## Scope notes
- Default ceiling is deliberately generous (120/10s) so a normal heavy run never
  false-trips; the loop guard (M116) already curbs per-run thrash, so this is the
  cross-run backstop, not the first line.
- Spend-rate/error-rate/repetition detectors are honest follow-ups (same Detector
  shape, different bus signal) — not shipped this turn, recorded in next.md.
