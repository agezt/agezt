# M260 — SDK milestone 3: human-in-the-loop approvals

## Why
After the run client (M258) and run inspection (M259), the next SDK surface an
embedding app needs is the **HITL approval** gate: when policy requires explicit
approval for a capability, the app should be able to list pending requests and
grant/deny them — building its own approval UI on the same mechanism `agt
approve` / `agt deny` use.

## What
- **`sdk/approvals.go`** — new:
  - `Approval` — a typed pending request: `ID`, `Capability`, `Tool`, `Reason`,
    `Actor`, `Input` (JSON-encoded when structured), `Timeout time.Time`.
  - `Client.PendingApprovals(ctx) ([]Approval, error)` — wraps `CmdApprovals`.
  - `Client.Approve(ctx, id, reason)` / `Client.Deny(ctx, id, reason)` — wrap
    `CmdDecide` with decision `grant` / `deny`.
  - `parseApprovals` maps `{"pending":[…]}` to typed values; `anyToString`
    renders a decoded input (string verbatim, structured value as compact JSON).

## Files
- `sdk/approvals.go` — `Approval`, `PendingApprovals`, `Approve`, `Deny`,
  `parseApprovals`, `anyToString` (new).
- `sdk/approvals_test.go` — 3 tests: parse (structured + string input, timeout,
  missing fields), empty result, and `anyToString` across nil/string/object/
  array/number (new).

## Verification
- `go test ./sdk/` — green; full suite **1842 → 1845** (+3), 67 packages,
  `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./sdk/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The mutating calls (`Approve`/`Deny`) and `PendingApprovals` run against a live
  daemon; the protocol-mapping (`parseApprovals`, `anyToString`) is unit-tested
  here.
- SDK arc: run client (M258), run inspection (M259), approvals (M260). Remaining
  natural slices: a streamed-events convenience and a documented `examples/`
  program that ties the surfaces together.
