# Phase Report — Milestone M32 (Targeted run cancellation)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/resilience). Fifth step on the resilience/observability
> axis (M28 → M29 → M30 → M31 → M32). M31 bounds a run's wall-clock
> automatically; M32 gives the operator a manual, surgical kill — one run, not
> the whole daemon.

## Why

The only existing way to stop an in-flight run was `agt halt`, which calls
`Kernel.HaltWith`: it cancels **every** in-flight run *and* sets the halt flag so
no new run starts until `agt resume`. That is the right tool for "stop the world"
(a runaway daemon, an emergency), but far too blunt for the common case — one
stuck run among many. An operator watching a single run hang on a dead provider
had to choose between pausing the entire daemon or waiting for M31's timeout (if
armed) / M28's boot abandon (next restart).

M32 adds the missing surgical option: cancel one run by correlation id, leave
everything else running. It rides entirely on the M30 terminal-event machinery —
a cancelled run's `context.Canceled` is already classified as
`task.failed(reason=canceled)` — so there is no new event, no new rendering, just
a new entry point into the existing live-run cancel registry.

## What shipped

- **`Kernel.CancelRun(corr string) bool` (`kernel/runtime/runtime.go`)** — looks
  up the run's `CancelFunc` in `k.runs` (the same registry `Halt` drains and
  `RunWith` populates), deletes the entry, and calls the cancel. Returns whether
  a live run matched. The cancel is the run context's own `CancelFunc`, so it
  cancels with `context.Canceled` (→ `reason=canceled`), distinct from M31's
  wall-clock `DeadlineExceeded` (→ `reason=timeout`). Crucially it does **not**
  touch `k.halted`, so the kernel stays open for business.
- **`CmdCancelRun` control-plane verb (`kernel/controlplane/`)** — `handleCancelRun`
  reads `args.correlation` (required) + `args.tenant` (optional, routed via the
  existing `kernelFor` seam), calls `CancelRun`, returns `{correlation, cancelled}`.
- **`agt runs cancel <correlation> [--json]` (`cmd/agt/runs.go`)** — wired into
  the `runs` dispatcher. Exit 0 when a live run was cancelled, 1 when none matched
  (already finished / unknown id) so scripts can branch; a missing id is a usage
  error (exit 2).

## Design decisions

- **Targeted, not global.** The whole point is to *not* be `Halt`. `CancelRun`
  leaves `k.halted` false and every other entry in `k.runs` untouched. The
  control-plane and runtime tests both assert `IsHalted()` stays false and that a
  *new* run is still accepted afterwards.
- **Reuse the registry and the terminal event.** No new bookkeeping: the cancel
  func is already stored per-correlation for `Halt`, and the resulting
  `context.Canceled` already maps to `reason=canceled` via M30's `failureReason`.
  M32 is a few lines of plumbing precisely because M28–M31 built the substrate.
- **Idempotent / race-safe.** `CancelRun` deletes the entry under the mutex;
  `RunWith`'s defer also deletes it. Double-delete and double-cancel are both
  harmless, so a cancel racing a natural finish just reports `false`.
- **`cancelled=false` is not an error.** Cancelling a run that already finished is
  a legitimate, common outcome (the operator was a beat late). The verb returns a
  clean `false`, and the CLI exits 1 with a human note — distinct from a transport
  error.
- **Tenant-routable.** Mirrors `handleRun`: a named tenant cancels within that
  tenant's isolated kernel, so one tenant can't cancel another's runs.

## Tests

- `kernel/runtime/runtime_test.go`:
  - `TestCancelRun_CancelsOneRunNotKernel` — a blocking run is cancelled by
    correlation; it returns `context.Canceled`, the journal records
    `task.failed(reason=canceled)`, `IsHalted()` stays false, and a *second* run
    is still accepted (proving the kernel wasn't halted) before being cleaned up.
  - `TestCancelRun_UnknownReturnsFalse` — unknown id and an already-finished run
    both report `false`.
- `kernel/controlplane/controlplane_test.go`:
  - `TestCancelRunViaControlPlane` — a streaming run's correlation is captured
    from its own event stream, `CmdCancelRun` cancels it (`cancelled=true`), the
    client's stream returns, and the kernel is not halted.
  - `TestCancelRun_UnknownAndMissingArg` — unknown correlation → `cancelled=false`
    (no error); missing `correlation` arg → request error.

Test count: **1221 → 1225**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean (the CRLF artifacts `gofmt -l` flags in
`cmd/agt/runs.go` are pre-existing and untouched by my additions).

## Live proof (mock provider + black-hole endpoint)

```
$ agt run "say hello" &          # hangs dialing 10.255.255.1:81
$ agt runs list 1
    started : … status: running             duration: —   iters: 0
$ agt runs cancel run-01KSZH79N27ND8M9846SVFPKNR
cancelled run run-01KSZH79N27ND8M9846SVFPKNR (it will terminate as failed/canceled)
$ agt runs list 1
  run-01KSZH79N27ND8M9846SVFPKNR
    started : … status: failed (canceled)   duration: 2.1s   iters: 0
$ agt runs stats        # daemon still serving, not halted
  failed    : 1
$ agt runs cancel run-DOESNOTEXIST
no in-flight run with correlation "run-DOESNOTEXIST" (already finished or unknown)
  → exit 1
```

The targeted cancel terminated exactly one run as `failed (canceled)`, the daemon
kept serving (`runs stats` responded, kernel un-halted), and cancelling a
non-existent run exited 1 — all end-to-end through the real control plane.

## What's next

The resilience/observability axis still has a few clean follow-ons:

1. **`agt runs stats --since <dur>`** (LOW) — a time-bounded health view; the
   failure / timeout / canceled terminal terms now make a windowed rate
   meaningful.
2. **Per-tool timeout** (MED) — finer than M31's per-run cap: one slow tool fails
   its single call (error fed back to the model) without killing the run.
3. **Cancel-on-disconnect** (MED) — optionally tie a `agt run` client's
   connection lifetime to its run, so Ctrl-C on the client cancels the run
   server-side (today the run continues; the per-connection ctx is the server
   root). Reuses `CancelRun`.
