# M183 — Nil-safe plugin Close

## Why
The plugin-host review (finding M3) noted `Close` dereferences process/pipe handles
without a nil guard:

```go
_ = p.writeRequest(...)              // p.stdin.Write — nil stdin panics
go func() { done <- p.cmd.Wait() }() // p.cmd nil panics
...
_ = p.cmd.Process.Kill()             // p.cmd.Process nil panics
```

On a `Plugin` whose child never finished starting (no `cmd`/`stdin`), each of these
nil-panics. In production this path is not reachable — `Spawn` returns the error and
never constructs the `Plugin` when `cmd.Start()` fails — but it's a latent footgun: a
future refactor that constructs or partially initializes a `Plugin` and then calls
`Close` (e.g. a new error path in `respawn`, or test/util code) would crash instead of
cleaning up. Defense-in-depth on the teardown path of an untrusted subsystem is cheap.

## What
`Close` now guards each handle:
- `if p.stdin != nil` before the best-effort shutdown write.
- `if p.cmd != nil && p.cmd.Process != nil` around the `Wait`/grace/`Kill` block.

It still marks the plugin dead and drains pending waiters (closing their channels) in
all cases, so a half-initialized plugin is cleaned up correctly rather than panicking.
No change to the normal (started) path.

## Tests
`kernel/plugin/close_test.go` (white-box) — `TestClose_SafeOnUnstartedPlugin`
constructs a `Plugin` with nil `cmd`/`stdin` and a registered pending waiter, then:
- `Close()` returns nil without panicking,
- the plugin is marked dead,
- the pending channel is closed (waiter unblocked),
- a second `Close()` is a no-op (idempotent), still no panic.

## Verification
- `go test ./...` — 1575 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — nil guards in `Close`.
- `kernel/plugin/close_test.go` — new.

## Follow-ups (same review, remaining)
M4 (process-group kill so a plugin's forked grandchildren don't survive `Close` —
cross-platform: Unix `Setpgid` + kill the group, Windows job object / process-group
flag). This is the last queued plugin-host review item; it touches the
platform-specific `makeChild` and warrants its own milestone. C1/H2/H3/H4/M1/M2/M3 are
now fixed (M177–M183).
