# M439 — Control plane: contain a panic in the pre-auth parse phase too

## Context
Resolving the deferred LOW: *"controlplane recoverConn-after-parse"* — the
per-connection panic guard was installed only after the request was read and
parsed, leaving the read/unmarshal phase uncontained.

## The gap
`handleConn` deferred `recoverConn` at line 382 — **after** `readBoundedLine`
(the bounded request read) and `json.Unmarshal`. So a panic during the read or
parse phase was not contained: it would propagate out of the connection goroutine
and crash the whole daemon (every in-flight run and channel with it). In practice
`encoding/json` does not panic on malformed input and `readBoundedLine` is a plain
bounded read, so this was latent rather than triggerable today — but it is a
defense-in-depth gap: any future parse/auth-phase code (a custom `UnmarshalJSON`,
added pre-dispatch logic) would be silently outside the guard, on the daemon's
most security-sensitive surface, which accepts bytes pre-authentication.

## The fix
Moved the `defer recoverConn` to the very top of `handleConn`, right after
`defer conn.Close()`, so the entire connection lifecycle — read, parse, auth,
dispatch — is contained. Because the request id isn't known until after parsing,
`recoverConn` now takes a `*Request` and reads `req.ID` **at panic time** rather
than receiving the id by value: an early panic carries an empty id, a post-parse
panic still carries the parsed id. `recoverConn` remains **directly** deferred
(not wrapped in a closure) so its `recover()` actually stops the panic — a
closure-wrapped call would silently fail to recover.

`recoverConn(conn, reqID string)` → `recoverConn(conn, req *Request)`; the
existing call site and the existing test updated accordingly.

## Verification
- **`kernel/controlplane/recover_test.go`**:
  - `TestRecoverConn_PanicBecomesErrorNotCrash` (existing) — panic → `RespError`,
    goroutine returns, id carried — updated to the `*Request` signature.
  - `TestRecoverConn_ReadsRequestIDAtPanicTime` (new) — defers `recoverConn(&req)`
    *before* assigning `req.ID`, then sets it and panics; asserts the response
    carries the late-assigned id. This locks in the late-binding that makes the
    top-of-handler defer correct (the id assigned after the defer is still
    reported).
  - **Negative control:** make `recoverConn` ignore the pointer (always empty id)
    → both id-carrying assertions FAIL (`response ID = ""`). Restored.
- **Gate:** staged (LF) blobs gofmt-clean, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2314** passing (was 2313;
  +1), `go test ./...` exit 0. CHANGELOG Reliability entry.

## Review status
The control plane's panic containment now spans the full connection lifecycle,
including the pre-auth read/parse phase.
