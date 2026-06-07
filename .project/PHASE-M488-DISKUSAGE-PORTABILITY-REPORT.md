# M488 — DiskUsage cross-platform build break (freebsd) + honest build constraints

## Context
Part of the OBJECTIVE-GATE arc. Added a **cross-compile build matrix** to the
verification battery (the prior arc only built `GOOS=linux`). It immediately found
a real portability defect.

## The bug
`kernel/pulse/diskusage_unix.go` carried `//go:build !windows`, claiming every
non-Windows OS. But its arithmetic assumed Linux/Darwin field types:

```go
bsize := uint64(st.Bsize)
return st.Bavail * bsize, st.Blocks * bsize, nil   // st.Bavail is int64 on FreeBSD
```

`syscall.Statfs_t.Bavail` is `uint64` on Linux/Darwin but **`int64` on FreeBSD**, so
`st.Bavail * bsize` is `int64 * uint64` — a compile error. `GOOS=freebsd go build
./...` failed outright. Two further non-Windows targets the constraint also claimed
could never compile: **OpenBSD** names the fields `F_bavail`/`F_bsize` (no `Bsize`),
and **NetBSD** has no `syscall.Statfs` at all (it needs `Statvfs1` via
`golang.org/x/sys`, which the project deliberately does not depend on — stdlib-only).

So the `!windows` constraint over-promised: the code compiled only on
linux/darwin/freebsd-after-fix, not "all non-Windows".

## The fix
1. **Type-safe arithmetic** — widen every operand to `uint64` explicitly. The
   conversion is a no-op where the field is already `uint64` and a valid widening
   where it is `int64`; block counts are inherently non-negative, so it is safe:
   ```go
   return uint64(st.Bavail) * bsize, uint64(st.Blocks) * bsize, nil
   ```
2. **Honest build constraint** — narrowed `diskusage_unix.go` to
   `//go:build linux || darwin || freebsd` (the `syscall.Statfs` + Bavail/Blocks/Bsize
   family).
3. **Graceful fallback** — new `diskusage_other.go`
   (`//go:build !windows && !linux && !darwin && !freebsd`) returns
   `errors.New("pulse: disk usage not supported on " + runtime.GOOS)`. Every
   `DiskUsage` caller already handles the error (`SetDiskFree` takes it as a function
   value; `main.go` guards with `if … err == nil`; `NewDiskObserver` skips on error),
   so OpenBSD/NetBSD/etc. simply report no disk metrics instead of failing the build.

## Test + negative control
DiskUsage is a thin syscall wrapper with no prior unit test; the meaningful
verification is the cross-compile type check itself. **Negative control:** reverting
just the `uint64()` conversions reproduced `invalid operation: st.Bavail * bsize
(mismatched types int64 and uint64)` under `GOOS=freebsd`; restoring them built
clean (byte-identical restore confirmed). The constraint+fallback half is proven by
OpenBSD/NetBSD going from FAIL → OK.

## Build matrix (after)
GREEN: linux/{amd64,arm64,386}, darwin/{amd64,arm64}, windows/{amd64,arm64},
freebsd/{amd64,arm64}, openbsd/amd64, netbsd/amd64.

Out of scope (architecturally unsupportable, not a defect): **plan9 / js / wasm** —
`kernel/plugin/proc_unix.go` needs `Setpgid`/`syscall.Kill` for process-group
management, and Agezt is fundamentally a subprocess-spawning plugin-host daemon
(plugin host, warden sandbox, control-plane process management). Those platforms
have no process model, so "building" there would mean stubbing the daemon down to a
non-functional shell — dead code for platforms it cannot run on. The supportable
server/desktop matrix is fully green.

## Verification / gate
- gofmt-clean on staged LF blobs; `go vet` clean under linux, freebsd, and openbsd
  (the build-tagged files only analyze under their own GOOS) and on the windows host;
  `staticcheck ./...` still 0.
- Full `go test ./...` exit 0 (host); cross-compile matrix green for all supportable
  targets; `go.mod`/`go.sum` unchanged.
