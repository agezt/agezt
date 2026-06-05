# M477 — Runtime: locked catalog read + close-all-stores on shutdown

Two defects in `kernel/runtime/runtime.go` from the scoped runtime/planner review.

## Fix 1 (MED) — data race on the `k.catalog` field
`RunWith`'s context-budget path read `k.catalog` **without** holding `k.mu`:

```go
if ctxBudget == 0 && k.cfg.ContextBudgetAuto && k.catalog != nil {
    if _, m := k.catalog.FindModel(model); m != nil { ... }
}
```

`ReloadCatalog` swaps that field under `k.mu` (`k.catalog = cat`), and the file
already exposes a correctly-locked accessor `Catalog()`. An operator running
`catalog sync` / `provider reload` while an auto-budget run starts is a genuine data
race (no happens-before; possible stale/torn pointer read). Fixed by reading through
`k.Catalog()`. (`k.mu` is not held at this point, so the accessor doesn't deadlock —
confirmed by the full suite.)

This is a benign-pointer race: the fix is correct-by-construction (it uses the
purpose-built locked accessor). A behavioral negative control isn't meaningful
offline — a benign pointer race rarely manifests without `-race` (which needs a C
compiler this environment lacks); the proper validator is CI's `-race`. The fix is
verified by reasoning + the full suite passing (no deadlock, no behavior change).

## Fix 2 (LOW-MED) — `Close()` leaked handles on the first error
`Close()` short-circuited:

```go
if err := k.state.Close(); err != nil { return err }
if err := k.memoryDir.Close(); err != nil { return err }
... // worldDir, skillDir
return k.journal.Close()
```

A non-nil error from any earlier store left the rest — notably `k.journal.Close()`,
which closes a real OS file descriptor — unclosed. A held journal handle blocks a
re-Open of the dir on Windows. Replaced with a `closeAll(...)` helper that invokes
every closer and `errors.Join`s the results.

### Test + negative control (Fix 2)
`kernel/runtime/closeall_internal_test.go`: `TestCloseAll_ClosesAllDespiteError` —
five fake closers, the 2nd and 5th erroring; asserts all five are invoked and the
joined error carries the failure.

**Negative control:** making `closeAll` short-circuit on the first error (the old
behavior) invoked only 2 of 5 — the test FAILED with `closeAll invoked 2 closers,
want 5 (later closers leaked)`. Restored; test passes.

## Verification / gate
- `kernel/runtime` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.

## Remaining (review) — not fixed
The review also noted a LOW: two concurrent `RunWith` calls sharing one
caller-supplied correlation id clobber the run registry (the doc contract requires
unique ids). Left documented.
