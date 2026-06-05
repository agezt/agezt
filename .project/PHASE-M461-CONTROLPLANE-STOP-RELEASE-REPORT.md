# M461 — Control plane: Stop() releases in-flight streaming handlers

## Context
`Server.Start(ctx)` launches `acceptLoop(ctx)`, and each accepted connection runs
`handleConn(ctx, conn)` with that same `ctx`. Long-lived streaming handlers
(`handleRun`, `handlePulseSubscribe`, `handlePlan`) block on `<-ctx.Done()` to end.
`Stop()` is documented as "safe to call from cleanup hooks even when Start was
driven by ctx".

## The bug (MED)
`Stop()` → `initiateShutdown()` only closed the **listener** and `s.done`; it never
cancelled the context the handlers wait on, and never closed accepted connections:

```go
func (s *Server) Stop() error {
    err := s.initiateShutdown() // closes listener + done only
    s.wg.Wait()                 // waits for every handleConn goroutine
    return err
}
```

Every `handleConn` goroutine is tracked in `s.wg`. A streaming handler ends only on
`ctx.Done()` (the Start ctx — not cancelled by Stop), a client disconnect, or the
per-connection read deadline (minutes). So when `Stop()` is used as the shutdown
trigger while a `pulse`/`run` stream is connected, `s.wg.Wait()` blocks until the
deadline — effectively a teardown hang. (When shutdown is driven by cancelling the
Start ctx instead, the same ctx unblocks the handlers, masking the bug on that
path.)

## The fix
Derive a serving context in `Start` and cancel it from `initiateShutdown`:

- New field `serveCancel context.CancelFunc` (guarded by `s.mu`).
- `Start`: `serveCtx, serveCancel := context.WithCancel(ctx); s.serveCancel = serveCancel`,
  and `acceptLoop(serveCtx)` (so every `handleConn` gets the derived ctx).
- `initiateShutdown`: after closing `done`/listener, call `serveCancel()`.

Because `serveCtx` is derived from the Start `ctx`, external ctx cancellation still
propagates as before; additionally a direct `Stop()` now cancels it, so streaming
handlers observe cancellation and `s.wg.Wait()` returns promptly. No leak: the
cancel is always invoked via `initiateShutdown` (which runs on either shutdown
path).

## Test + negative control
`kernel/controlplane/stop_test.go`: `TestStop_ReleasesInFlightStream` — holds a
`pulse` subscription open via `StreamUntilCancel`, then calls `srv.Stop()` **without
cancelling the Start ctx** and asserts it returns within 3 s.

**Negative control:** disabling the cancel (`serveCancel != nil && false`) made
`Stop()` hang on the in-flight stream — the test reported `srv.Stop() did not
return` and FAILED (3 s timeout). Restored; test passes.

## Provenance
Second item from the scoped controlplane/plugin code review (M460 was the first, the
HIGH plugin write deadlock). The review's remaining item — a bounded
`callWithProgress`/`Close` registration TOCTOU — is LOW (the racing caller unwinds
on its own invoke-timeout, not a hang) and is left documented.

## Verification / gate
- `kernel/controlplane` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
