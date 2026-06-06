# M531 — Pin the control-plane request-size limit inclusive boundary

## Context
Targeted mutation of `kernel/controlplane`'s pre-auth DoS guard, `readBoundedLine`
(M188) — the bounded reader that caps a control-plane request line before authentication
(the token is inside the line, so any local client reaching the loopback port can stream
bytes pre-auth). Whole-package `go-mutesting` is intractable here (~10k LOC), so this is a
targeted negative-control sweep of one security-critical function. `GOMAXPROCS=3`.

## The genuine gap (closed)
```
if len(buf)+len(chunk) > max { return nil, errRequestTooLarge }
```

Same inclusive-boundary shape as plugin `readFrame` (M509). `request_limit_test.go` covers
under-cap (`hello\n` vs 1024), a flood well over (`5000` vs `1000`), EOF-mid-line, and
multi-chunk reassembly — but no request sitting *exactly* on the cap. The fuzz invariant
only asserts `len(line) <= max`, which a `>=`-mutant (returning early at exactly max)
still satisfies. So `> max → >= max` survived: a request whose length exactly fills the
cap would be wrongly rejected as "request too large".

## Fix
Added `TestReadBoundedLine_ExactlyMaxAccepted`: a request of exactly `max` bytes
(including the trailing newline) is accepted and returns `max` bytes; `max+1` returns
`errRequestTooLarge`.

## Negative control (manual, CPU-capped)
`len(buf)+len(chunk) > max → >= max`: FAIL (the exactly-at-max request is rejected).
Restored byte-for-byte (`git diff --ignore-all-space` on server.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Control-plane coverage so far
Three security-critical control-plane primitives are now verified/pinned by negative
control: `tokenIsPrimary` (M529), `tenantTokenAllows` (M530), and `readBoundedLine`'s
inclusive cap (M531). The ~40 command handlers remain covered by the 71 test files but not
exhaustively mutation-tested (intractable at scale).
