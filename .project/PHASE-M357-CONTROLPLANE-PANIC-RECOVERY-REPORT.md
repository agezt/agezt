# M357 — Control-plane per-connection panic recovery

## Why
Priority-A reliability/resilience fix, found by a panic-containment audit (grep
`recover()` vs the goroutines that handle requests). `kernel/agent`,
`kernel/plugin/host`, and the plugin SDK already recover from panics — but the
control-plane's per-connection goroutine (`handleConn`, spawned per accepted
connection) did **not**. In Go, an unrecovered panic in *any* goroutine crashes
the whole process — so a panic in any command handler (a latent nil-deref, an
out-of-range, an unexpected edge case in less-travelled code) would take down the
entire daemon: every in-flight run, every channel poller, the web UI, all of it.

The control plane is loopback + token-authed, so this isn't remote-unauthenticated,
but a single malformed-yet-authed request that tickles a handler bug should be
contained to its own connection — not a daemon-wide DoS. `net/http` already does
per-request recovery; the control plane's custom TCP protocol needed the same.

## What
Production fix + lock-in test.
- **`kernel/controlplane/server.go`** — new `(*Server).recoverConn(conn, reqID)`,
  deferred in `handleConn` immediately after the request is parsed (so `reqID` is
  known). On a panic it writes a `RespError{"internal error"}` to the caller and
  returns, so the goroutine unwinds cleanly instead of propagating. Best-effort:
  if the connection is already broken the error write is dropped; the load-bearing
  guarantee is that the **process survives**.
- **`kernel/controlplane/recover_test.go`** —
  `TestRecoverConn_PanicBecomesErrorNotCrash`: over a `net.Pipe`, a goroutine
  defers `recoverConn` then panics; the test reads the `RespError` (with the
  request ID carried through) from the other end and confirms the goroutine
  completed — i.e. the panic was recovered, not propagated.

## Verification
- `go test ./kernel/controlplane -run RecoverConn -v` — passes.
- `gofmt -l` clean; `go vet ./kernel/controlplane/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2091** passing (was 2090; +1), `go test ./...` exit
  0. `go.mod`/`go.sum` unchanged. CHANGELOG updated (Reliability).

## Scope notes
- The recovery returns a generic `internal error` to the client (no stack trace
  leaked over the wire). A future enhancement could journal the panic + stack for
  `agt why`/diagnosis; that needs a new event kind and is noted, not bundled here.
- The other request-handling goroutines were checked: `agent.Run` recovers
  internally (so a panicking tool is already contained before it reaches a channel
  or the control plane), and `net/http`-based servers (REST/OpenAI API, web UI)
  recover per request via the stdlib. The control-plane TCP handler was the one
  un-guarded request goroutine.
