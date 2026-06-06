# M538 — Mutation testing mcpbridge: pin the MCP-frame cap inclusive edge

## Context
Fourth `plugins/` target: `plugins/external/mcpbridge` (the bridge to external MCP
servers — stdio child or remote SSE stream, both *untrusted* peers). Its `readBoundedLine`
(M185) caps a single newline-delimited frame so a hostile/buggy server can't OOM the
bridge. `GOMAXPROCS=3`.

## The genuine gap (closed)
```
if len(buf)+len(chunk) > max { return nil, errMCPFrameTooLarge }
```

The third instance of this inclusive-max bounded-read guard (after plugin `readFrame` M509
and control-plane `readBoundedLine` M531). `limits_test.go` covers normal frames,
multi-chunk under-max, EOF-mid-line, and three over-max floods — but no frame sitting
*exactly* on `max`. So `> max → >= max` survived: a frame whose total length exactly fills
the cap would be wrongly torn down as "frame too large" — a legitimate 16 MiB JSON-RPC
payload from a well-behaved server dropped at the boundary.

## Fix
Added `TestReadBoundedLine_ExactlyMaxAccepted`: a frame of exactly `max` bytes (incl. the
newline) is accepted and returns `max` bytes; `max+1` returns `errMCPFrameTooLarge`.

## Negative control (manual, CPU-capped)
`len(buf)+len(chunk) > max → >= max`: FAIL (the exactly-at-max frame is rejected). Restored
byte-for-byte (`git diff --ignore-all-space` on limits.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Cross-package pattern note
This is the THIRD copy of the identical `len(buf)+len(chunk) > max` bounded-read guard
(plugin host frames, control-plane requests, MCP-server frames). All three had the same
unpinned inclusive-max edge; all three are now pinned. The shape is a recurring,
copy-pasted DoS-guard idiom — worth a shared helper + shared test if it recurs again.

## Plugins-tree mutation progress
file (M535), http (M536), shell (M537), mcpbridge frame cap (M538). Remaining mcpbridge
surface (JSON-RPC dispatch, SSE event assembly, transport teardown) and channel/provider
adapters remain.
