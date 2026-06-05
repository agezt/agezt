# M438 — Cadence: backstop a hung firing so its schedule can't wedge

## Context
Resolving the highest-value previously-deferred item (carried since the cadence
review): *"MED — cadence in-flight guard never cleared on an unbounded run hang."*

## The bug
`kernel/cadence` prevents overlapping runs of the same schedule with an in-flight
guard: `fireDue` does `running.LoadOrStore(entry.ID, …)` and skips an entry whose
ID is already present; `fireOne` clears it with `defer e.running.Delete(ent.ID)`.

But the guard is cleared only when `fireOne` **returns**, and `fireOne` blocks on
`e.run(ctx, …)` — which (in the daemon) is `k.RunWith(daemonCtx, …)` with **no
per-firing deadline**. The agent loop's own bounds (HTTP/provider/tool timeouts)
normally make a run terminate, but if a run ever hangs — a wedged provider/tool
that ignores its own bounds, a deadlock — `fireOne` never returns, the
`running` entry is never deleted, and **that schedule never fires again**: a
silent, permanent stall of one schedule (MED), with no error surfaced. The only
recovery is a daemon restart.

## The fix
Added an optional backstop deadline on the engine:
- `Engine.RunTimeout time.Duration` (0 = no deadline, the historical behavior).
  `fireOne` wraps the run's context in `context.WithTimeout(ctx, RunTimeout)` when
  it is set, so a ctx-respecting run is cancelled at the deadline, `fireOne`
  returns, the `defer running.Delete` clears the guard, and the schedule recovers
  on its next slot. (A run that ignores ctx entirely cannot be unblocked in Go by
  any means; the realistic hang — a ctx-aware provider/tool call — is covered.)
- The daemon (`buildCadence`) sets a **1 h default** backstop, overridable via
  `AGEZT_SCHEDULE_RUN_TIMEOUT` (a Go duration; `0`/`off` disables). 1 h is
  generous for any reasonable agentic scheduled run while bounding a pathological
  hang. The new env var is registered in the control-plane config allowlist
  (`kernel/controlplane/config.go`, alphabetically) — the config-coverage test
  enforces this.

The field is set before `Start`, so the ticker goroutine reads it race-free.

## Verification
- **`kernel/cadence/cadence_test.go`** `TestEngine_RunTimeoutClearsInflightGuard`:
  a ctx-respecting run that blocks on `<-ctx.Done()` (would hang ~forever) with
  `RunTimeout=50ms`; `fireOne` is run on a goroutine and asserted to return within
  a 5 s guard, after which the in-flight guard is confirmed cleared.
  - **Negative control:** disable the `WithTimeout` wrap (`&& false`) → the test
    FAILs at the 5 s guard ("fireOne did not return — RunTimeout failed to bound a
    hung run"). Restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2313** passing (was 2312;
  +1), `go test ./...` exit 0. CHANGELOG Reliability entry. (Initial full-suite run
  caught the missing config-allowlist entry — `TestConfigEnvVars_CoversCmdAgeztReads`
  — which was then added; a good demonstration of that guard working.)

## Review status
The cadence subsystem's deferred MED is resolved. The standing-order companion
(`kernel/standing`) already bounds its fires via `safeFire`; both proactive
timers now contain a hung firing rather than wedging.
