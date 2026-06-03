# M261 — SDK milestone 4: streamed-event helpers

## Why
`Client.RunStream` (M258) hands the caller raw `*Event` values whose useful data
lives in a JSON `Payload`. Without helpers, a consumer must import
`kernel/event` for the Kind constants and hand-unmarshal each payload to render
progress. This adds small, typed decoders for the three common cases so the
streaming surface is usable without touching kernel internals.

## What
- **`sdk/events.go`** — new:
  - `TokenText(ev) (string, bool)` — the streamed text delta of an `llm.token`
    event (false for other kinds / empty deltas), for rendering the answer live.
  - `ToolCall(ev) (string, bool)` — the tool name from a `tool.invoked` event.
  - `IsTerminal(ev) bool` — true for `task.completed` / `task.failed`, the run's
    last event.

## Files
- `sdk/events.go` — `TokenText`, `ToolCall`, `IsTerminal` (new).
- `sdk/events_test.go` — 3 tests covering each helper across the matching kind,
  a non-matching kind, empty payload, and nil (new).

## Verification
- `go test ./sdk/` — green; full suite **1845 → 1848** (+3), 67 packages,
  `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./sdk/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The helpers import `kernel/event` only for the Kind constants — internal to the
  SDK; consumers call the helpers and never import kernel packages.
- SDK arc: run client (M258), run inspection (M259), approvals (M260), event
  helpers (M261). The natural capstone is a documented `examples/` program that
  uses Dial → RunStream (+ TokenText) → Runs together.
