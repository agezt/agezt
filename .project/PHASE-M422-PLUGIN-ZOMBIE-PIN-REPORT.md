# M422 — Plugin host: zombie reaping + pin-path bypass

## Context
A security/reliability review of the plugin host (subprocess lifecycle + RPC) and the
warden sandbox. The **warden was found clean** (capped-output hard bound verified,
Linux process-group kill on timeout, no shell interpretation, empty child env so
secrets aren't inherited). The plugin host had one HIGH reliability bug and one
LOW/MED security bug.

## Fixes

### HIGH — self-exiting plugin never reaped (zombie) — `kernel/plugin/host.go`
`cmd.Wait()` was called in exactly one place: inside `Close()`. But `Close()`
short-circuited at the top when `p.dead` was set, and the read loop's death path
(`markDead`, fired on plugin EOF/crash) set `dead` **without** ever calling `Wait`.
So a plugin that exited or crashed on its own — or was reloaded — was never reaped:
on Linux it became a zombie held for the daemon's lifetime, and a crash-looping or
repeatedly-reloaded plugin accumulated one per death until the process table filled.

Fix: a dedicated per-process waiter goroutine (`startWaiter`) is launched at every
(re)spawn and is the *sole* owner of `cmd.Wait()`, closing `p.waitDone` once the child
is reaped — guaranteeing reaping on every death path (self-exit, crash, kill).
`Close` was rewritten to wait on (or force, via a process-group kill after the grace
period) `waitDone` instead of starting its own `Wait` goroutine, so `Wait` is never
called twice and a dead-but-still-alive plugin (abnormal `markDead`) is still killed
and reaped. `markDead` deliberately does not kill (it would race a concurrent Reload
swapping `p.cmd`); the kill happens in `Close`.

### LOW/MED — pin verification bypass via a bare-name path — `kernel/plugin/pin.go`
`VerifyPin`/`HashFile` opened the binary with `os.Open` (CWD-relative), while
`exec.Command` resolves a bare name (no separator) via `$PATH`. For
`AGEZT_PLUGINS="t=mytool"` with a pin, the daemon hashed `./mytool` but executed
`$PATH/mytool` — a same-named decoy in CWD could satisfy the pin while a different
binary ran. New `resolvePluginPath` resolves a bare name to its absolute `$PATH`
location (via `exec.LookPath`) once in `Spawn`, before both the hash check and exec,
and the resolved path flows into `p.cfg` so Reload/respawn stay consistent. Separator
paths and unresolvable names are returned unchanged (already consistent / fail closed).

## Verification
- **`kernel/plugin/reap_test.go`** `TestStartWaiter_ReapsSelfExitedChild`: spawns the
  test binary as a self-exiting child (portable helper-process idiom), confirms the
  waiter reaps it (`waitDone` closes, `cmd.ProcessState != nil`) with no `Close`.
  - **Negative control:** waiter without `cmd.Wait()` → `ProcessState` stays nil
    (zombie) → FAIL. Restored.
- **`kernel/plugin/pinpath_test.go`** `TestResolvePluginPath`: separator path and
  unresolvable name unchanged; a bare name on `$PATH` resolves to an absolute path.
  - **Negative control:** resolution removed → bare name unchanged → FAIL. Restored.
- The full plugin suite (Close/Reload/deadtool/flood/callback lifecycle) still passes,
  confirming no deadlock or double-`Wait` regression.
- **Gate:** `gofmt -l` clean, `go vet` clean (incl. `GOOS=linux go vet`), `GOOS=linux
  go build ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2277** passing (was
  2274; +3). CHANGELOG Reliability + Security entries.

## Review status
This closes the plugin-host findings. The warden sandbox (`kernel/warden`) was
reviewed and found clean. The Windows process-tree teardown gap (grandchildren not
killed) remains a documented, by-design limitation (Linux is the first-class target;
a Job Object follow-up is tracked in the code).
