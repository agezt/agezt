# M258 — Public Go SDK, milestone 1: the run client

## Why
With the bug frontier exhausted, the chosen next direction is the first big
feature from the original vision list: an **SDK**. Today the only way to embed
Agezt in a Go program is to import internal kernel packages
(`kernel/controlplane`, `kernel/event`) and hand-build the control-plane CmdRun
argument/result maps — the same plumbing `cmd/agt` does inline. That's an
unstable surface (internal types, untyped maps, wire-protocol knowledge). This
milestone introduces a public, stable, ergonomic client package so a developer
can run an intent in a few lines.

## What
- **`sdk/sdk.go`** — new package `github.com/agezt/agezt/sdk`:
  - `Dial(baseDir) (*Client, error)` — connects to the local daemon ("" → the
    default base via `internal/paths`). File-based, so a Dial error means "no
    daemon recorded", not "unreachable".
  - `Client.Run(ctx, intent, opts...) (*Result, error)` and
    `Client.RunStream(ctx, intent, onEvent, opts...)` — run an intent through the
    control plane's CmdRun, returning a typed `Result{Answer, CorrelationID,
    Model, Iterations, CostUSD}`.
  - Functional options: `WithModel`, `WithTenant`, `WithSystem`, `WithTimeout`,
    `WithTools` (explicit empty allow-list is distinct from omitted),
    `WithImages` (data: URLs), `WithMaxCostUSD`.
  - `Event = event.Event` type alias for the stream callback; `DefaultBaseDir()`
    exposes the base resolution.
  - The arg-building (`buildRunArgs`) and result-parsing (`parseResult`) match
    `agt run` byte-for-byte (timeout as a Go duration string, tools/images as
    JSON arrays, `max_cost` in microcents where $1 = 1e9, result keys
    answer/correlation_id/model/iters/spent_mc).

The SDK wraps the existing control-plane client; it adds no dependencies
(stdlib + the existing internal packages only).

## Files
- `sdk/sdk.go` — the package (new).
- `sdk/sdk_test.go` — 8 tests: arg building (defaults, all options,
  explicit-empty vs omitted tools, non-positive cost ignored), result parsing
  (full + missing-fields), Dial (reads runtime files / errors with none) (new).

## Verification
- `go test ./sdk/` — green; full suite **1832 → 1840** (+8), now **67 packages**
  (the new `sdk`), `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./sdk/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- `Run`/`RunStream` are exercised end to end against a live daemon; the pure
  arg-building and result-parsing (the protocol-mapping logic) are unit-tested
  here, and `Dial` is tested against on-disk runtime files (no daemon needed).
- This is milestone 1 of the SDK arc. Natural next slices: a streaming-events
  convenience (filter/decode common event kinds), read-side helpers (`Runs`,
  `Why`, approvals), and a documented example under `examples/`. Each is a tight
  follow-up reusing this client.
