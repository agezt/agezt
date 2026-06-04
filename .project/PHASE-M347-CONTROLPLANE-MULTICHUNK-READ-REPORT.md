# M347 — Control-plane bounded reader multi-chunk reassembly coverage

## Why
Priority-A coverage on the control-plane's untrusted-input framing.
`readBoundedLine` (server.go, M188) reads one newline-delimited request line while
bounding the total to `maxRequestBytes` (16 MiB), reading in `ReadSlice` chunks and
accumulating across `bufio.ErrBufferFull` so a line longer than the reader's buffer
is reassembled whole. The existing `TestReadBoundedLine_Request` covered the
under-cap single-read, the over-cap rejection, and EOF-mid-line — but **not** the
multi-chunk `ErrBufferFull` accumulation path, which is the trickiest branch (the
`continue` that copies each chunk out before the next read so the returned slice
stays stable) and the one any real request larger than bufio's 4 KiB buffer
actually hits. A truncation/corruption bug there would silently mangle large but
valid control-plane requests while the suite stayed green.

## What
Test-only, white-box (`package controlplane`). Added to
`kernel/controlplane/request_limit_test.go`:
- **`TestReadBoundedLine_MultiChunkReassembly`** — a `bufio.NewReaderSize(r, 16)`
  (the 16-byte bufio minimum) feeding a 100-byte line forces `ReadSlice` to return
  `ErrBufferFull` ~6 times; with a 1024 cap the line is well under the bound. The
  test asserts the returned slice is the **complete** 101-byte line (100 chars +
  newline), proving the cross-chunk reassembly — a buffer-boundary truncation bug
  would return only 16 bytes.

## Verification
- `go test ./kernel/controlplane -run ReadBoundedLine -v` — both bounded-reader
  tests pass.
- `gofmt -l` clean; `go vet ./kernel/controlplane/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2072** passing (was 2071; +1), `go test ./...` exit
  0. `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — the reassembly already worked; this pins it. With the
  over-cap rejection (request_oversize_test.go) and EOF/under-cap cases already
  covered, `readBoundedLine`'s branches are now all exercised.
- Completes the untrusted-input framing audit on the control plane alongside the
  size-bound family closed in M346.
