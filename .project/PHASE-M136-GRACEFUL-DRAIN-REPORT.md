# M136 — Graceful shutdown drain (SPEC-08 §3.1)

## Why
Completes the deployment story (M134 health probes + M135 metrics). SPEC-08 §3.1
calls for shutdown to "stop accepting new work (drain)". But `Kernel.Close()`
opens with `k.Halt()` — *"cancel any in-flight runs first"*. So on `agt shutdown`
/ SIGTERM, in-flight runs were **cancelled**, not drained. For a rolling restart
or k8s rollout (now realistic since the daemon has readiness probes), that kills
work mid-flight every deploy.

## What
Readiness-based draining — the standard k8s/LB pattern — inserted before the
existing halt:

1. **Flip readiness first.** A shared `draining atomic.Bool` is set true; the
   `/readyz` probe (M134) now reports 503 `"draining"`. A load balancer / readiness
   probe pulls the instance from rotation, so no *new* external traffic arrives,
   while the process stays alive.
2. **Wait, bounded, for in-flight runs.** `drainWait(k.ActiveRuns, timeout)` polls
   until `ActiveRuns()` hits 0 or the timeout elapses. `AGEZT_DRAIN_TIMEOUT`
   (default 15s; `0` = don't wait → the old immediate-halt behavior) bounds it, so
   a stuck run can't hang shutdown — on timeout it falls through to the existing
   cancel + halt.
3. **Then** the unchanged `cancel()` + halt-loop + `k.Close()`.

`drainWait` is extracted as a pure helper so the wait/timeout logic is unit-tested
without standing up the daemon. The drain flag is shared with `buildRESTAPI`'s
readiness closure via an `*atomic.Bool` — clean race semantics between the
shutdown goroutine and HTTP handlers.

## Files
- `cmd/agezt/main.go` — `draining atomic.Bool`; threaded into `buildRESTAPI`
  (readiness reports "draining"); shutdown sequence flips it + `drainWait` before
  halting; `drainWait` helper; `sync/atomic` import.
- `kernel/controlplane/config.go` — `AGEZT_DRAIN_TIMEOUT` added to the env
  inventory (the M127 drift guard caught the new read — working as designed).
- `cmd/agezt/main_test.go` — `TestDrainWait` (idle → true immediately; active
  decrements to 0 → true; always-busy + short timeout → false; `timeout<=0` →
  busy false / idle true).

## Live proof (offline mock)
```
GET /readyz (before)        → 200 {"status":"ready"}
$ agt shutdown
  daemon log: "agezt: shutting down (requested via agt shutdown)..."
  → process exits cleanly (0 in-flight runs, so the bounded drain wait is skipped)
```
The drain-with-in-flight-runs path (wait then succeed / time out, and the
readiness "draining" flip) is covered by `TestDrainWait`; the instant mock can't
hold a run in flight long enough to observe the wait live.

## Verification
- 55 packages `ok`, **FAIL 0**; **1432 tests** (incl. the M127 env-inventory drift
  guard, which now passes with `AGEZT_DRAIN_TIMEOUT` registered).
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched files clean under LF.
