# M419 — HTTP slow-loris timeouts (network-facing DoS hardening)

## Context
A security review of the network-facing control surfaces (control-plane TCP server,
tenant isolation, web UI) found the auth paths, tenant path-traversal defenses, CSRF
posture, and read-only enforcement all correct and well-tested. It surfaced one HIGH
reliability/DoS bug in the daemon wiring.

## The bug
`cmd/agezt/main.go` built all three HTTP servers with **zero timeouts**:
```go
srv := &http.Server{Handler: webui.New(...).Handler()}   // web UI
srv := &http.Server{Handler: api.Handler()}              // OpenAI-compat API
srv := &http.Server{Handler: rest.Handler()}             // REST
```
Go's `net/http` applies no default deadlines, so a client that opens a connection and
sends one header byte every N seconds (or never completes the request line) holds a
server goroutine + connection indefinitely. With no connection cap, N such
connections exhaust file descriptors / memory and wedge the web UI **and** the agent's
API surfaces. The control-plane TCP server already defends against this (10-minute
read deadline + 16 MiB bounded read), which made the HTTP servers the weak link.

## The fix
New `newGuardedHTTPServer(h)` helper builds every HTTP surface with
`ReadHeaderTimeout` (10s) and `IdleTimeout` (120s). `WriteTimeout` is **deliberately
left unset**: the web UI's `/events` SSE stream and the OpenAI-compat streaming
completions are long-lived responses that a `WriteTimeout` would kill mid-flight.
`ReadHeaderTimeout` is the correct slow-loris mitigation — it bounds only the
pre-handler header read, not the streaming body. The three call sites now use the
helper (also DRYs the construction).

## Verification
- **`cmd/agezt/httpserver_test.go`** `TestGuardedHTTPServer_SlowLorisTimeouts`: the
  helper returns a server with non-zero `ReadHeaderTimeout` == the const, non-zero
  `IdleTimeout` == the const, and `WriteTimeout` == 0 (so SSE/streaming survives).
  - **Negative control:** omitting `ReadHeaderTimeout` in the helper → the test FAILs
    (`ReadHeaderTimeout = 0s`). Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2269** passing (was 2268; +1). CHANGELOG
  Reliability entry added.

## Other review finding (deferred — not exploitable)
`kernel/controlplane/server.go`: `recoverConn` is deferred *after* the per-request
`readBoundedLine` + `json.Unmarshal`, so a panic during framing/unmarshal would be
uncovered. Currently NOT reachable — `bufio` read, `append`, and `json.Unmarshal` into
a fixed struct do not panic on attacker bytes (malformed JSON returns an error). Left
as-is to avoid churning the critical accept path; flagged as defense-in-depth should a
custom decoder ever be added ahead of the handler switch.

## Review status
This closes the one exploitable finding from the control-surface review. Control-plane
token auth (constant-time compare, no pre-auth-reachable command, loopback-only bind,
bounded reads/deadlines), tenant isolation (id pattern rejects traversal, fail-closed,
constant-time token compare, per-tenant base dirs), and the web UI (constant-time
bearer compare on every route incl. SSE, POST-only mutations, token-not-in-cookie so
CSRF doesn't apply, `Referrer-Policy: no-referrer`, embed-only assets, exhaustive
read-only GET enforcement) were all found correct and well-tested.
