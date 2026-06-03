# M257 — Bound per-message reads in the ACP server and client

## Why
M256 bounded the acpagent tool's *accumulation* of streamed chunks, but a single
oversized message could still exhaust memory one layer down: both the ACP server
(`kernel/acp`, driven by an IDE over stdio) and the ACP client (driving an
external agent subprocess) read with `json.Decoder.Decode`, which buffers an
entire JSON value with no size cap. A malicious or buggy peer sending one
multi-gigabyte message would OOM the process before any higher-level cap saw it.
This was the "known limitation" flagged after M256; on a closer look it is
**low-risk to fix** because ACP is newline-delimited JSON — both ends write via
`json.Encoder`, which appends a newline per message — so a bounded line scanner
is wire-compatible.

## What
- **`kernel/acp/acp.go`** — added `maxMessageBytes = 8 MiB`, `newBoundedScanner`
  (a `bufio.Scanner` whose token buffer is capped, so an over-cap line surfaces
  as `bufio.ErrTooLong`), and `scanMessage` (reads the next non-blank
  newline-delimited message into a value, returns `io.EOF` at clean end). The
  `Server` now holds a `*bufio.Scanner` instead of a `*json.Decoder`; `Serve`
  reads via `scanMessage`.
- **`kernel/acp/client.go`** — the `Client` likewise holds a `*bufio.Scanner`;
  its response/notification read loop uses `scanMessage`.

`json.RawMessage` fields copy their bytes on unmarshal, so reusing the scanner's
buffer across messages is safe.

## Files
- `kernel/acp/acp.go` — scanner + `scanMessage` helpers; `Server` read path
  (edited).
- `kernel/acp/client.go` — `Client` read path (edited).
- `kernel/acp/bound_test.go` — 3 tests: an oversized message errors (not
  buffered), a normal message reads with leading blank lines skipped and a clean
  EOF, and an at-cap message still reads (the bound rejects only what exceeds it)
  (new).

## Verification
- `go test ./kernel/acp/` — green, including the existing server/client
  round-trip tests (confirming the line-scanning framing is wire-compatible);
  full suite **1829 → 1832** (+3), 66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/acp/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- 8 MiB per message is generous for JSON-RPC control messages and prompt chunks
  while still bounding a pathological one. The acpagent tool's own answer cap
  (60 KiB, M256) is a separate, smaller bound on the *relayed* text.
- ctx-cancellation semantics are unchanged — the read still blocks between
  messages, exactly as `json.Decoder.Decode` did.
- Closes the memory-DoS surface on ACP end to end (per-message bound here +
  accumulation bound in M256).
