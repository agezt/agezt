# M184 — Process-group teardown for plugins (no orphaned grandchildren)

## Why
The plugin-host review (finding M4, the last queued item) noted that `Close` kills
only the direct child:

```go
_ = p.cmd.Process.Kill()   // direct child only
```

A plugin frequently is a thin wrapper that forks its own children — a shell launching
a binary, a Python process spawning a subprocess. When the host kills only the plugin,
those grandchildren are orphaned and keep running: a resource leak, and for an
untrusted plugin a persistence / sandbox-escape flavour (work continues after the host
believes it tore the plugin down).

## What
Place each plugin in its own process group at spawn, and signal the whole group at
teardown. The mechanism is OS-specific, so it lives behind a small platform split:

- **`kernel/plugin/proc_unix.go`** (`//go:build !windows`):
  - `setProcessGroup(cmd)` sets `SysProcAttr.Setpgid = true`, making the child a group
    leader (its pgid == its pid).
  - `killProcessTree(cmd)` sends `SIGKILL` to the negative pid (the whole group),
    falling back to killing just the child if the group signal fails.
- **`kernel/plugin/proc_windows.go`** (`//go:build windows`):
  - `setProcessGroup` is a no-op; `killProcessTree` kills the direct child. Reliable
    whole-tree teardown on Windows needs a Job Object, which the host does not yet set
    up — a documented limitation. Linux is the daemon's first-class deployment target
    (the build rhythm gates every change on `GOOS=linux`).

Wiring:
- `makeChild` (pin.go) calls `setProcessGroup(cmd)` so every spawned/respawned plugin
  is isolated.
- `Close`'s grace-timeout branch calls `killProcessTree(p.cmd)` instead of
  `p.cmd.Process.Kill()`.

The graceful-shutdown path (shutdown request + grace period) is unchanged; the group
kill only matters when a plugin must be force-terminated.

## Tests
- `kernel/plugin/proc_test.go` (cross-platform, runs everywhere): `killProcessTree` is
  nil/unstarted-safe (no panic on `nil` or a never-started cmd); `makeChild` still
  yields a runnable command with the expected args.
- `kernel/plugin/proc_unix_test.go` (`//go:build !windows`): `makeChild` sets
  `Setpgid`. This asserts the core isolation on the Unix target; it is validated on
  Linux CI and is NOT run on the Windows dev box (where `setProcessGroup` is a no-op).
  Its compilation under the Unix build was verified here via `GOOS=linux go vet`.

### Local-proof note
The behavioural grandchild-reaping guarantee is a Unix-runtime property; it cannot be
executed on this Windows dev box (the daemon's process-group code only runs on Linux,
which can't be exercised natively here). Verified locally: both platforms BUILD and
`vet` clean, the Unix test type-checks under `GOOS=linux`, and the existing live plugin
spawn/close tests (which now route through the new `makeChild`/`Close` paths) still
pass — so there is no regression. The Unix kill-the-group semantics follow the standard
`Setpgid` + negative-pid `SIGKILL` pattern.

## Verification
- `go test ./...` (Windows) — 1577 passing, 0 failing. (Linux CI runs one more: the
  Unix-gated `TestMakeChild_SetsProcessGroup`.)
- `go vet ./kernel/plugin/` clean; `GOOS=linux go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `go build ./...` and `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/proc_unix.go`, `kernel/plugin/proc_windows.go` — new platform split.
- `kernel/plugin/pin.go` — `makeChild` calls `setProcessGroup`.
- `kernel/plugin/host.go` — `Close` uses `killProcessTree`.
- `kernel/plugin/proc_test.go`, `kernel/plugin/proc_unix_test.go` — new.

## Plugin-host security arc — complete
With M184 the entire plugin-host review is addressed: C1 (M177), H2 (M178), H3 (M179),
H4 (M180), M1 (M181), M2 (M182), M3 (M183), M4 (M184). Remaining nice-to-haves the
review marked LOW (pin TOCTOU, plugin-controlled log strings) are documented in their
respective reports and left per the stated threat model.

## Windows follow-up
Whole-tree teardown on Windows via a Job Object (assign the child to a job at spawn;
terminating the job kills the tree). Larger, Windows-specific, and lower priority given
the Linux-first deployment target.
