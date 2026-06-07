# M509 — Mutation testing plugin: pin readFrame's inclusive max-size boundary

## Context
Twentieth package in the mutation pass: `kernel/plugin` (the plugin host — subprocess
spawn, stdio framing, AllowedTools allowlist, limit defaults). Run with `GOMAXPROCS=3`
(CPU-capped). Score 0.609, 143/366 survived; working tree restored clean after the run
(`git checkout -- .` + `rm -f report.json`, per the Windows go-mutesting hazard).

## The genuine gap (closed)
`readFrame(r *bufio.Reader, max int)` (host.go) reads a newline-delimited frame while
bounding total size to guard against an OOM flood from a malicious plugin:

```
if len(buf)+len(chunk) > max { return nil, errFrameTooLarge }
```

The limit is **inclusive** — a frame whose total length (including the trailing newline)
is exactly `max` must be accepted. `frame_test.go` covered normal frames, multi-chunk
under-max, over-max rejection, and EOF-mid-line — but **no case sat exactly on `max`**.
So the `> max` boundary survived `> → >=`: under the mutant a frame that exactly fills
the limit is wrongly rejected as `errFrameTooLarge`, an off-by-one that would drop
legitimate maximum-size frames.

## Fix
Added `TestReadFrame_ExactlyMaxAccepted` to `frame_test.go` (`package plugin`, internal):
- a frame of exactly `max` (64) bytes including the newline → accepted, `len == max`;
- a frame of `max+1` bytes → `errFrameTooLarge`.

## Negative control (manual, CPU-capped)
- `> max → >= max`: FAIL (the exact-max frame is rejected; `err != nil`).
Restored byte-for-byte (`git diff --ignore-all-space` on host.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty packages (M490–M509)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin
— plus the controlplane primary-token auth gate verified solid. Recurring closeable
class confirmed again: an inclusive boundary that end-to-end tests skip because their
inputs never land exactly on the limit.
