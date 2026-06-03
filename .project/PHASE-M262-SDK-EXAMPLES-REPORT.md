# M262 — SDK milestone 5 (capstone): runnable example + godoc examples

## Why
The SDK now has a run client (M258), run inspection (M259), approvals (M260),
and event helpers (M261). The capstone an adopter needs is a worked example —
both a copy-paste program and the godoc usage snippets that appear on each
method.

## What
- **`examples/agezt-run/main.go`** — a runnable command that demonstrates the
  whole SDK flow: `Dial("")` → `RunStream` (printing the answer live via
  `TokenText`, noting tools via `ToolCall`) → print the `Result` (correlation,
  model, iterations, cost) → `Runs(ctx, 5)` to list recent runs. Flags for
  `-model` and `-timeout`. This is the new 68th package; it builds clean on every
  platform.
- **`sdk/example_test.go`** — godoc `Example` functions (`ExampleClient_Run`,
  `ExampleClient_RunStream`, `ExampleClient_Runs`, `ExampleClient_PendingApprovals`)
  in `package sdk_test`, so they read as real consumer code and appear in godoc.
  They are compiled by `go test` (compile-checking the public API surface) but
  not executed (no `// Output:` — they need a live daemon).

## Files
- `examples/agezt-run/main.go` — runnable demo (new package).
- `sdk/example_test.go` — godoc examples (new).

## Verification
- `go test ./sdk/` — green (examples compile-checked); `go build ./examples/...`
  clean; full suite still **1848** (godoc examples are `Example` funcs, not
  `Test` funcs, so the tally is unchanged), now **68 packages** (the new
  `examples/agezt-run`), `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./examples/... ./sdk/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The example program and godoc examples need a running daemon to execute, so
  they are compile-verified rather than unit-run — standard for examples that
  touch external resources.
- **SDK arc complete**: run client (M258), run inspection (M259), approvals
  (M260), event helpers (M261), and a worked example (M262). An embedding Go
  program can now drive Agezt end to end through a stable, documented surface
  without importing kernel internals or speaking the control-plane wire protocol.
