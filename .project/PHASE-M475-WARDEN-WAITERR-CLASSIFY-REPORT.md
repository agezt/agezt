# M475 — Warden: don't swallow non-ExitError engine failures on a non-zero exit

## Context
After `cmd.Wait()`, the warden runner classifies the error: an `*exec.ExitError`
means the process ran (a non-zero exit belongs in `Result.ExitCode`, not as an
engine error), while anything else (failed launch, I/O error, `WaitDelay`
abandonment after a kill) is a genuine engine failure to return.

## The bug (MED)
The classification was gated on the exit code:

```go
if err != nil && !res.TimedOut && res.ExitCode == 0 {
    var ee *exec.ExitError
    if errors.As(err, &ee) { return res, nil }
    return res, fmt.Errorf("warden: exec %q: %w", spec.Argv[0], err)
}
return res, nil
```

The `&& res.ExitCode == 0` condition is logically wrong: the type check alone should
decide. Because a non-`ExitError` failure almost always coincides with a non-zero /
`-1` exit code (a killed or abandoned process), the guard skipped exactly the cases
it should catch — so a genuine engine failure with a non-zero exit was returned as
`(res, nil)`, hiding it from the caller (the common case is correct only by accident:
a plain non-zero exit is an `*exec.ExitError`, which should be absorbed, and is).

## The fix
Extract a type-based `classifyWaitErr(err, timedOut, argv0)` and drop the exit-code
gate:

```go
func classifyWaitErr(err error, timedOut bool, argv0 string) error {
    if err == nil || timedOut { return nil }
    var ee *exec.ExitError
    if errors.As(err, &ee) { return nil }
    return fmt.Errorf("warden: exec %q: %w", argv0, err)
}
```

This surfaces strictly *more* failures than before (never fewer): the only behavior
change is that a non-`ExitError` engine failure is now returned regardless of exit
code. `nil`, timed-out, and `ExitError` (any exit code) outcomes are unchanged.

## Test + negative control
`kernel/warden/classify_test.go` (white-box): `TestClassifyWaitErr` — nil → no error;
a timed-out run → absorbed; a non-`ExitError` engine failure → surfaced (the fix).
The `ExitError`-absorbed path is covered end-to-end by the existing `exit 7`
integration test (which expects no error and `ExitCode == 7`), still passing.

**Negative control:** making `classifyWaitErr` return `nil` for the non-`ExitError`
case (reproducing the swallow) made the test FAIL with `a non-ExitError engine
failure must be surfaced, got nil (swallowed)`. Restored; test passes.

## Provenance
From the scoped review of kernel/warden (capbuf cap arithmetic, env handling /
nil-Env→empty, argv no-shell-interpolation, process-group kill + WaitDelay teardown
all reviewed CLEAN — no sandbox escape, secret leak, output-cap bypass, or surviving
process/goroutine).

## Verification / gate
- `kernel/warden` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
