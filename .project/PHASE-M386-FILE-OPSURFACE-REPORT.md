# M386 — Correct stale file-tool op-scope comment + op-surface lock-in (priority-C)

## Audit (read-vs-code)
`plugins/tools/file/file.go` package doc said:

```
// Scope (M1.a): read, write, list, search. A future `patch` op (unified
// diff) is deferred; for M1 the model can use write to rewrite a whole
// file or append.
```

**Verified stale:** the tool's own schema enum (and the `Invoke` switch) handle
**nine** ops — `read, write, append, list, search, stat, delete, replace, glob` —
and `replace` (M114) is a surgical find/replace edit, contradicting "the model
can use write to rewrite a whole file" / "patch deferred". The model-facing
`Definition().Description` was already correct ("Prefer `replace` for small
edits…"); only the developer-facing package comment had rotted (listed 4 of 9
ops + an out-of-date deferral).

## What
- **`plugins/tools/file/file.go`** — rewrote the doc to list all nine ops and
  note `replace` exists (unified-diff `patch` still deferred), pointing at the
  new lock-in test.

## Verification (lock-in)
- **`plugins/tools/file/file_test.go`** `TestFile_EveryAdvertisedOpIsDispatched`:
  parses the tool's OWN schema `op` enum and invokes each op, asserting none
  falls through to `"unknown op"`. This keeps the advertised surface and the
  dispatch switch in lockstep, so neither the enum, the switch, nor the doc can
  silently drift again. (Individual op behaviour is covered by the existing
  TestWriteReadRoundtrip/Append/List/Search/Stat/Delete/… tests.)
- **Negative control:** adding a `"phantom"` op to the schema enum (no switch
  case) → the test FAILs (`advertised … but not dispatched: "unknown op
  \"phantom\""`); restored `file.go` byte-identical.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2168** passing (was 2167; +1). No CHANGELOG (developer comment + test only;
  the model-facing description was already accurate).

## Scope notes
- Category-C method (M384): when a comment lists a deferred/scope set, verify the
  WHOLE set against code, not just the headline item.
