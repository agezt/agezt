# M480 — Runtime: reject a duplicate live correlation id in RunWith

## Context
`RunWith(ctx, corr, intent)` registers a run's cancel func in `k.runs[corr]` under
`k.mu`, and a deferred `delete(k.runs, corr)` removes it when the run ends. `Halt`
and `CancelRun` cancel runs by looking up `k.runs[corr]`. `corr` is caller-supplied.

## The bug (LOW)
Nothing guarded against two concurrent runs sharing one `corr`:

```go
k.runs[corr] = cancel   // second call overwrites the first's cancel
...
defer func() { ... delete(k.runs, corr) ... }()   // first to finish deletes the OTHER's entry
```

Two concurrent `RunWith` with the same id: the second's `k.runs[corr] = cancel`
overwrites the first's entry, then whichever finishes first deletes `k.runs[corr]` —
removing the survivor's entry. The remaining run becomes uncancellable by
`Halt`/`CancelRun`, and the `k.fanout[corr]` tally is similarly clobbered. The doc
contract says each run gets its own id, so this is a defensive guard — hence LOW.

## The fix
Reject a correlation that is already running, under the same lock that registers it:

```go
k.mu.Lock()
if k.halted { k.mu.Unlock(); return "", ErrHalted }
if _, running := k.runs[corr]; running {
    k.mu.Unlock()
    return "", fmt.Errorf("runtime: correlation %q is already running", corr)
}
...
k.runs[corr] = cancel
k.mu.Unlock()
```

Check and registration are both under `k.mu`, so concurrent duplicates are mutually
exclusive — the loser gets a clear error instead of corrupting the registry.

## Test + negative control
`kernel/runtime/runtime_test.go`: `TestRunWith_RejectsDuplicateCorrelation` — a mock
provider whose `OnRequest` blocks holds the first run in-flight (registered in
`k.runs`); a second `RunWith` with the same id must return an "already running"
error.

**Negative control:** disabling the check (`running && false`) let the second call
proceed into the run (blocking in the held provider) — the test deadlocked and hit
its timeout. Restored; test passes.

## Provenance
The last open item from the kernel/runtime review (M477 fixed the catalog race +
Close leak; this closes the runtime list).

## Verification / gate
- `kernel/runtime` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
