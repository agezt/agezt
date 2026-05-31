# Phase Report — Milestone M35 (Cancel-on-disconnect)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/resilience). Eighth step on the resilience/observability
> axis (M28 → … → M35). M32 lets an operator cancel a run by id; M35 makes the
> most natural cancel — Ctrl-C on the `agt run` you're watching — Just Work.

## Why

`agt run` streams a run's events and blocks until it finishes. If the operator
Ctrl-Cs that client (or it's killed), the run kept going server-side: the
control-plane `handleRun` uses the daemon's *root* context, not the per-connection
one, so a dropped client left an orphaned, headless run churning until it finished
on its own, hit a timeout (M31), or was abandoned on the next boot (M28). The
intuitive expectation — "I stopped watching, stop the work" — wasn't met.

M35 ties the run's life to its client connection (opt-in), reusing M32's
`CancelRun` so a disconnect terminates the run cleanly as `failed (canceled)`.

## What shipped

- **Disconnect watcher in `handleRun` (`kernel/controlplane/server.go`)** — when
  enabled, after starting the run goroutine the handler spawns a watcher that
  clears the connection's read deadline and does a single blocking `conn.Read`.
  The client sends nothing after its request, so the read unblocks **only** when
  the connection closes; at that point the watcher calls `k.CancelRun(corr)` — the
  exact path `agt runs cancel` uses (M32) — cancelling the run with
  `context.Canceled` → `task.failed(reason=canceled)` (M30). When the run instead
  finishes normally, `handleConn`'s `defer conn.Close()` closes the conn, the
  watcher's read returns, and `CancelRun` is a harmless no-op (the run is gone).
- **`Server.SetCancelOnDisconnect(bool)` + `cancelOnDisconnect` field** — mirrors
  the existing `SetPulse`/`SetTenants` injection pattern; read per-request so it
  can be toggled before any run.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_CANCEL_ON_DISCONNECT=on`; boot
  banner `cancel-on-disc. : …`.

## Design decisions

- **Off by default.** A backgrounded `agt run &` keeps its client process alive, so
  its connection stays open and it is unaffected even when the feature is on — only
  a *genuinely gone* client (Ctrl-C / killed / crashed) drops the connection. Still,
  to avoid surprising anyone relying on fire-and-forget semantics across odd
  network conditions, the behaviour is opt-in.
- **Clear the read deadline in the watcher.** `handleConn` sets a 10-minute read
  deadline for the request read; left in place, the watcher's blocking read would
  return a *timeout* at 10 minutes and falsely cancel a long but healthy run.
  Clearing it (`SetReadDeadline(time.Time{})`) means the read returns only on a
  real close. Writes are unaffected (they use no deadline).
- **Reuse `CancelRun`, don't invent a path.** The disconnect is just another
  trigger for the M32 cancel; routing through `k.CancelRun(corr)` means the
  terminal event, the `failed (canceled)` rendering, and the un-halted-kernel
  guarantee are all inherited. A finished run → `CancelRun` returns false → no-op,
  so the watcher is safe to fire unconditionally on close.
- **No goroutine leak.** The watcher blocks on `conn.Read`; the connection is
  always closed when `handleConn` returns (run finished, errored, or cancelled), so
  the read always unblocks and the watcher always exits.

## Tests

`kernel/controlplane/controlplane_test.go` (driving a real server + client over the
loopback socket, using `StreamUntilCancel` which closes the conn on ctx cancel):
- `TestCancelOnDisconnect_Enabled` — a hung run's client connection is dropped; the
  run terminates as `task.failed(reason=canceled)`.
- `TestCancelOnDisconnect_DisabledByDefault` — with the feature off, a dropped
  client does **not** cancel the run (it stays live and is cancelled manually to
  clean up), proving the default is unchanged.

Test count: **1229 → 1231**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (black-hole endpoint)

```
$ AGEZT_CANCEL_ON_DISCONNECT=on agezt …
  cancel-on-disc.  : on (a dropped `agt run` client cancels its run)

$ agt run "say hello" &        # hangs dialing 10.255.255.1:81
$ agt runs list 1 → status: running
$ kill -9 <agt-run-client-pid>  # simulate Ctrl-C / killed client
$ agt runs list 1
    started : … status: failed (canceled)   duration: 2.3s   iters: 0
```

Killing the client process terminated the run server-side as `failed (canceled)`
within ~2s — the disconnect watcher detected the closed connection and routed
through `CancelRun`, end-to-end.

## What's next

The resilience axis is now very broad. Remaining clean follow-ons:

1. **`agt runs list --since <dur>`** (LOW) — mirror M33's window on the list view;
   lift the `since_ms` parse + `StartedUnixMS >= cutoff` filter into a shared
   helper used by both stats and list.
2. **Tool-timeout observability** (LOW) — a `tool.timeout` event or a reason tag on
   `tool.result` so `agt runs stats` can surface a per-tool timeout rate distinct
   from ordinary tool errors.
3. **`failed`/`canceled` breakdown in `agt runs stats`** (LOW) — split the failure
   term by `reason` (error / max_iters / canceled / timeout) so an operator sees
   *why* runs are failing, not just how many.
