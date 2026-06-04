# M338 — Warden capBuffer (output memory bound) unit coverage

## Why
Priority-A test coverage on a correctness/safety-critical primitive. The warden
process-isolation layer (SPEC-06 §2) caps tool output at `MaxOutputBytes` so a
runaway or hostile command can't exhaust memory by emitting gigabytes — the cap
is enforced by `capBuffer` (`kernel/warden/capbuf.go`), a tail-truncating
`io.Writer` that retains only the most-recent `max` bytes. Its `Write` has two
distinct truncation branches (drop the whole existing buffer + head of the new
chunk, vs. drop only the head of the existing buffer) and off-by-one-prone
arithmetic (`off := max(n-keep, 0)`, `c.buf[drop:]`, the overlapping
`append(c.buf[:0], c.buf[drop:]...)` in-place shift). Yet it was only exercised
end-to-end through `warden.Run` (one `TestRun_OutputTruncated`); the primitive
itself, and especially its boundary cases, had no direct test.

A subtle bug here is a real memory-safety / correctness risk (wrong tail, or a
buffer that creeps past the cap). This milestone pins the invariant directly so
it can't regress. No production change.

## What
Test-only. New **`kernel/warden/capbuf_test.go`** (white-box `package warden`, 9
tests):
- under-cap fast path keeps everything, not flagged truncated;
- exact-fit write (`len+n == max`) stays fast path, not truncated;
- single write larger than cap keeps the correct tail (`drop >= len(buf)` branch)
  and `Write` still reports all bytes consumed (io.Writer contract);
- second write crossing the boundary, `drop < len(buf)` → **default branch**
  (keep tail of existing buffer + all of new chunk);
- second write with `drop >= len(buf)` → whole existing buffer dropped;
- empty/`nil` write is a no-op (returns 0, no state change);
- non-positive `max` falls back to `DefaultMaxOutputBytes`;
- **tail-invariant fuzz**: a sequence of varying-size chunks (some < cap, some >
  cap) drives every branch repeatedly and asserts the buffer always equals the
  last `max` bytes of everything written, never exceeding the cap;
- a single huge write retains exactly `max` bytes (the memory bound).

## Verification
- `go test ./kernel/warden -run CapBuffer -v` — all 9 pass; the tail-invariant
  test confirms the most-recent-output property across both truncation branches.
- `gofmt -l` clean; `go vet ./kernel/warden/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2059** passing (was 2050; +9), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- White-box test (the primitive is unexported); it coexists with the existing
  black-box `package warden_test` file in the same directory.
- No behaviour change — `capBuffer` already truncated correctly; this locks the
  contract. The Linux namespace/seccomp backend (`warden_linux.go`) remains
  build-only verifiable on this Windows host (covered by `GOOS=linux build`).
