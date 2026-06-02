# M188 — Bounded control-plane request read (pre-auth DoS)

## Why
The control plane's connection handler read each request line with an unbounded reader:

```go
reader := bufio.NewReader(conn)
line, err := reader.ReadBytes('\n')   // grows without limit until '\n'
```

Two things make this dangerous:
1. **It is pre-authentication.** The token is *inside* the request, so the bytes must be
   read before `tokenIsPrimary`/tenant auth runs. Any local process that can connect to
   the loopback control port — no token required — reaches this read.
2. **`ReadBytes('\n')` is unbounded.** A client that streams bytes and never sends a
   newline drives the daemon's buffer to grow until it OOMs. The 10-minute read deadline
   bounds *time*, not *memory* — many gigabytes flow over loopback well within it.

This is the same unbounded-read OOM class fixed in the plugin host (M177) and mcpbridge
(M185), now on the control socket and reachable pre-auth.

## What
- Added `maxRequestBytes = 16 << 20` (16 MiB — far above any legitimate command, even a
  large inline run prompt), `errRequestTooLarge`, and a `readBoundedLine` helper (the
  same `ReadSlice` chunk-accumulation shape used elsewhere) in the controlplane package.
- `handleConn` now reads the request via `readBoundedLine(reader, maxRequestBytes)`. An
  over-cap request gets a `request too large` error response and the connection is
  dropped, instead of unbounded allocation. Behaviour for normal-size requests is
  unchanged.

## Tests
- `kernel/controlplane/request_limit_test.go` (white-box): `readBoundedLine` round-trips
  an under-cap line, rejects an unterminated over-cap flood with `errRequestTooLarge`,
  and returns partial bytes + `io.EOF` mid-line.
- `kernel/controlplane/request_oversize_test.go` (live): dials the real listener raw and
  streams 17 MiB with no newline and no token; the server answers `request too large`
  (read back over the socket) rather than hanging or growing unbounded. Proves the
  pre-auth path is bounded end-to-end (~0.05 s).

## Verification
- `go test ./...` — 1591 passing, 0 failing.
- `go vet ./kernel/controlplane/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/controlplane/server.go` — `maxRequestBytes`, `errRequestTooLarge`,
  `readBoundedLine`, bounded read in `handleConn`.
- `kernel/controlplane/request_limit_test.go`, `request_oversize_test.go` — new.

## Note
The client-side reads in `client.go` (reading server responses) are left unbounded:
`agt` trusts the daemon it connected to, and bounding there would only guard against a
compromised daemon attacking its own CLI — out of scope for this control-socket
hardening. The two control-plane review items (M187 constant-time token, M188 bounded
request) are now both shipped.
