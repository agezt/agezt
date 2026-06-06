# M537 — Mutation testing the shell tool: pin negative-timeout fallback

## Context
Third `plugins/` target: `plugins/tools/shell` (the command-execution tool). Execution
itself is delegated to `kernel/warden` — the sandbox/timeout/output-cap engine, already
verified solid by mutation testing (M495). The shell tool is thin glue: input validation,
timeout-precedence selection, profile request, and stream rendering. `GOMAXPROCS=3`.

## Triage
The empty-command guard (`in.Command == ""` killed), exit-code rendering
(`res.ExitCode != 0` killed), truncation marker, stream combine, and run-correlation
stamping are all covered by the existing suite. Execution security lives in warden (M495).

## The genuine gap (closed)
Timeout precedence: `DefaultTimeout`, overridden by `t.Timeout > 0`, then by
`in.TimeoutMS > 0`:

```
if in.TimeoutMS > 0 { timeout = time.Duration(in.TimeoutMS) * time.Millisecond }
```

`> 0` and `!= 0` agree on positive and zero inputs but differ on a **negative**
`timeout_ms`: the guard correctly ignores it (falls back to the default), but `!= 0` would
forward it as a *negative* duration to warden. A negative timeout can be interpreted as
"no deadline" — silently disabling the timeout runaway-guard from a single malformed tool
argument. The mutation `> 0 → != 0` survived because no test used a negative `timeout_ms`.

## Fix
Added `TestShell_NegativeTimeoutMSFallsBackToDefault` (using `capturingWarden` to inspect
the built Spec): `timeout_ms: -1` must yield `Limits.Timeout == DefaultTimeout`.

## Negative control (manual, CPU-capped)
`in.TimeoutMS > 0 → != 0`: FAIL (a negative timeout_ms is forwarded as a negative
duration). Restored byte-for-byte (`git diff --ignore-all-space` on shell.go empty);
passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Plugins-tree mutation progress
file (M535), http (M536), shell (M537) — three tools. Their security cores (path
containment, SSRF allowlist, warden execution) are solid; the closeable gaps were
inclusive/sign boundaries on input validation. Remaining: mcpbridge, channel adapters,
provider adapters.
