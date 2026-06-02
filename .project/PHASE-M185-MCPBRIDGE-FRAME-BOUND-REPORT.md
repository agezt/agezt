# M185 — Bounded reads from untrusted MCP servers (mcpbridge)

## Why
`mcpbridge` is the external-MCP plugin: it bridges the agezt host to MCP servers over
two transports, and BOTH read newline-delimited frames from a peer the bridge does not
control:

- **stdio** (`stdio_transport.go:79`): `t.stdout.ReadBytes('\n')` from a spawned MCP
  server subprocess.
- **SSE** (`sse_transport.go:186`): `br.ReadString('\n')` from a remote HTTP
  `text/event-stream` body.

`bufio`'s `ReadBytes`/`ReadString` grow their buffer without limit until a newline or
EOF. A hostile or buggy MCP server that writes bytes but never emits `\n` (or emits one
pathologically large line) drives the bridge process to OOM. This is exactly the
plugin-host C1 class (fixed in M177) one trust boundary further out — the MCP server is
even less trusted than a local plugin. The SSE transport had a second unbounded path:
`dataBuf += "\n" + value` accumulates a multi-line event's `data` with no cap, so a
server streaming endless `data:` lines without a dispatching blank line grows it
without limit even when each line is individually small.

## What
A stdlib-only bounded line reader shared by both transports.

- **`limits.go`** — `readBoundedLine(r *bufio.Reader, max int) ([]byte, error)`: reads
  in buffer-sized `ReadSlice` chunks, copies each out before the next read, and returns
  `errMCPFrameTooLarge` once the accumulated frame would exceed `max` (16 MiB,
  `maxMCPFrameBytes`). Same proven shape as the plugin host's `readFrame`.
- **stdio**: `readLoop` captures `maxFrame := maxMCPFrameBytes` once at start and reads
  via `readBoundedLine`. An over-cap frame surfaces through the existing
  `onTransportDead` path — the transport dies, the bridge survives.
- **SSE**: `readLoop` likewise reads each line via `readBoundedLine`, AND bounds the
  accumulated event `data`: appending a `data:` value that would push `dataBuf` past
  `maxFrame` tears the stream down (`signalEndpoint` + `onTransportDead`).

`maxMCPFrameBytes` is a `var` (not a const) only so tests can lower it to drive the
bound cheaply; each read loop captures it once at start (before its goroutine reads
it), so a set-before-construct test override is race-free via the goroutine-creation
happens-before edge.

The bridge's own stdin read (`main.go:188`, from the agezt host) is deliberately left
unbounded: the host is the trust root for the bridge, and it already bounds its own I/O
(M177); this milestone targets the untrusted-server-facing reads.

## Tests
- `limits_test.go` (white-box): `readBoundedLine` over normal sequential frames, a line
  larger than bufio's 4 KiB buffer returned whole (multi-chunk path), overflow rejected
  for a small cap / across chunks / a 1 MiB unterminated flood (the OOM scenario), and
  EOF-mid-line returning the partial bytes with `io.EOF`.
- `sse_limit_test.go` (live): using the existing `mockMCPServer`/`captureDeliver`
  harness with `maxMCPFrameBytes` temporarily lowered to 1 KiB, a server that streams a
  4 KiB `data:` event causes the transport to die with `errMCPFrameTooLarge` (asserted
  via `errors.Is` through the wrap) — not hang or grow unbounded. Construction still
  succeeds because the small endpoint event is under the cap.

## Verification
- `go test ./...` — 1585 passing, 0 failing.
- `go vet ./plugins/external/mcpbridge/` clean.
- `gofmt -l` clean on my added lines (CRLF-normalized). Note: `sse_transport.go` carries
  a PRE-EXISTING gofmt artifact (the `httpClient:` field alignment + a trailing blank
  line) that is present in `HEAD` and untouched by this change — left per the
  pre-existing-artifact policy.
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `plugins/external/mcpbridge/limits.go` — new (`maxMCPFrameBytes`, `errMCPFrameTooLarge`,
  `readBoundedLine`).
- `plugins/external/mcpbridge/stdio_transport.go` — bounded `readLoop`.
- `plugins/external/mcpbridge/sse_transport.go` — bounded per-line read + bounded
  `dataBuf` accumulation.
- `plugins/external/mcpbridge/limits_test.go`, `sse_limit_test.go` — new.
