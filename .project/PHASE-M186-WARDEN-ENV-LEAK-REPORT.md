# M186 — Warden nil-env no longer leaks the daemon environment

## Why
A security review of the warden (the process-isolation layer that runs untrusted
agent-invoked commands) found a HIGH secret-leak. `Spec.Env` is documented (warden.go):

> Env is the *exact* environment the child sees. Nil = empty environment (most
> restrictive); pass os.Environ() to inherit everything (least restrictive).

But `Run` set the child's environment with a bare assignment:

```go
cmd.Env = spec.Env
```

Go's `os/exec` treats `cmd.Env == nil` as **"inherit the parent process's
environment."** So a `Spec` with `Env` left nil — exactly the value a caller picks when
trusting the documented "most restrictive" default — ran the untrusted child with the
**entire daemon environment**: provider API keys, tokens, `AWS_*`, `HOME`, etc. The
documented safe default was in fact the most dangerous one.

This was not hypothetical: `kernel/pulse/observers.go` constructs its probe-runner
`warden.Spec` with `Env: nil` for operator-configured probe commands, so every probe
child inherited all daemon secrets.

## What
`Run` now translates a nil `Env` to an explicit empty slice, making the documented
contract the actual behavior:

```go
if spec.Env == nil {
    cmd.Env = []string{}
} else {
    cmd.Env = spec.Env
}
```

`cmd.Env = []string{}` is Go's documented way to request an empty environment (distinct
from nil). A caller that genuinely wants inheritance must now pass `os.Environ()`
explicitly — which is also what `Spec.Env`'s doc already says.

## Tests
`kernel/warden/env_test.go`:
- `TestRun_NilEnvDoesNotInheritParentEnv` — sets a unique sentinel var in the parent
  (test) process, runs a child that echoes that var with `Env: nil`, and asserts the
  sentinel value is **absent** from the child's stdout. Without the fix the child
  inherits the var and echoes its value, failing the test.
- `TestRun_ExplicitEnvIsPassedThrough` — positive control: runs with an explicit env
  containing a sentinel and asserts the child echoes it. Proves the echo mechanism
  surfaces a present var (so the leak test's "absent" is meaningful) and guards against
  regressing explicit-env pass-through.

Both portable across Linux/macOS/Windows via an `echoEnvArgv` helper.

## Verification
- `go test ./...` — 1587 passing, 0 failing.
- `go vet ./kernel/warden/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/warden/warden.go` — nil-Env → empty translation in `Run`.
- `kernel/warden/env_test.go` — new.

## Follow-ups (from the same warden review)
The review surfaced further items worth their own milestones, by severity:
- **CRITICAL** — non-Linux timeout kills only the direct child; grandchildren orphaned
  (Windows needs a Job Object; macOS/BSD can reuse the POSIX `Setpgid` + `kill(-pgid)`
  pattern already on Linux).
- **HIGH** — rlimits are applied to the leader only, after Start, so any forked child
  (the documented `sh -c` shell-wrapper pattern) runs unbounded; real enforcement needs
  setrlimit-before-exec or cgroup v2 on the subtree.
- **HIGH** — a failed group-kill (`cmd.Cancel`) is unaudited; a child calling `setsid()`
  escapes the group kill.
- **MEDIUM** — `prlimit`-setfailed is mislabeled as a "limit exceeded" event; no
  `Argv[0]`/`WorkDir` validation (poisonable `PATH`, "scoping" overstates confinement).
- **LOW** — downgrade-event dedup can drop a later run's dedicated downgrade alert
  (mitigated: the per-run `warden.executed` event always carries `downgraded`);
  `SetBus` writes `e.bus` locklessly (documented "before first Run" contract).
