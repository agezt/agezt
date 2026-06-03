# M259 — SDK milestone 2: run inspection (`Client.Runs`)

## Why
M258 gave the SDK a run client. The next thing an embedding app needs is to see
what the agent has done — run history, status, cost. This adds the read-side
primitive: list recent runs as typed values, reading the daemon's journal
without starting anything.

## What
- **`sdk/runs.go`** — new:
  - `RunInfo` — a typed run summary: `CorrelationID`, `Intent`, `Status`
    (completed/failed/running/abandoned), `Reason` (failure tag), `ParentCorrelation`
    (sub-agent lead), `Started time.Time`, `Duration time.Duration`,
    `Iterations`, `CostUSD`, `Model`.
  - `Client.Runs(ctx, limit) ([]RunInfo, error)` — wraps the control-plane
    `CmdRunsList` (newest first; `limit <= 0` uses the daemon default).
  - `parseRuns` converts the wire result (`{"runs":[…]}` with `started_unix_ms`
    / `duration_ms` / `spent_mc`) into idiomatic `time.Time` / `time.Duration` /
    USD — more ergonomic than the raw maps the CLI renders.

## Files
- `sdk/runs.go` — `RunInfo`, `Runs`, `parseRuns` (new).
- `sdk/runs_test.go` — 2 tests: full parse (times, duration, cost, parent,
  failure reason, missing-time zero values) and the empty/absent cases (new).

## Verification
- `go test ./sdk/` — green; full suite **1840 → 1842** (+2), 67 packages,
  `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./sdk/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- `Runs` is exercised end to end against a live daemon; the protocol-mapping
  (`parseRuns`) is unit-tested here with representative result maps.
- SDK arc so far: run client (M258), run inspection (M259). Next slices:
  approvals (HITL grant/deny), a streamed-events convenience, and a documented
  `examples/` program.
