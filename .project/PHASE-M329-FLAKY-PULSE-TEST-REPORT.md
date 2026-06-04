# M329 — De-flake TestStartStopsOnContextCancel

## Why
`kernel/pulse.TestStartStopsOnContextCancel` failed intermittently (~1 in 5 full
`go test ./...` runs; reliably green when run alone). A flaky test erodes trust in
the suite and would make CI non-deterministic. It surfaced during M327's
verification.

## Root cause
The test started the pulse engine (5 ms cadence), slept 25 ms, called `cancel()`,
then slept a **fixed 20 ms** and read `before := Status().Beats`, slept 30 ms more,
and asserted the beat count was unchanged. The engine's loop goroutine simply
`return`s on `ctx.Done()` with no stop signal, so the test can't know when it has
actually exited — it just hopes 20 ms is enough for cancel to propagate. Under a
loaded parallel test run, cancel propagation plus an in-flight tick can outlast
that sleep, so a beat lands between the baseline read and the final read, failing
the assertion. Pure test-timing fragility — not an engine bug.

## Fix
- **`kernel/pulse/engine_test.go`** (test-only — no production change): after
  `cancel()`, poll until the beat count is **stable across a window wider than the
  cadence** (`4 * cadence`), bounded by a 2 s deadline. A cancelled engine
  quiesces within one tick, so beats converge quickly; a still-running engine
  would keep incrementing and never stabilize (→ deadline failure). Once stable,
  a final window confirms the count stays frozen. This replaces the fixed-sleep
  guess with a deterministic convergence wait that tolerates a loaded scheduler.
  Also added an explicit pre-cancel check that the engine actually beat, so the
  test can't pass vacuously.

## Verification
- The de-flaked test passed **50 consecutive runs** (`-count=20` then `-count=30`)
  including while 8 parallel copies ran — previously it failed ~1/5.
- Full suite green across **6 consecutive `go test ./...` runs** (previously ~1/5
  failed on this test); exit 0; `gofmt -l` clean; `go vet` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged. Test count unchanged (2022 — an
  existing test was hardened, not added).

## Scope notes
- No production code changed; engine shutdown behaviour is unchanged (the loop
  already exits correctly on `ctx.Done()` — the test just couldn't observe it
  deterministically).
- A cleaner long-term option is a `Wait()`/done-channel on the engine so callers
  (and tests) can synchronously await shutdown, but that's a production API
  addition; the test-only convergence wait fixes the flake with zero behavioural
  risk.
