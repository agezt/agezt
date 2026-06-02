// SPDX-License-Identifier: MIT

package main

// Bounded reads from untrusted MCP servers (M185). Both transports —
// stdio (a spawned MCP server's stdout) and SSE (a remote HTTP
// event-stream) — read newline-delimited frames from a peer the bridge
// does not control. A plain bufio ReadBytes/ReadString grows its buffer
// without limit until a newline or EOF, so a server that writes bytes
// but never emits '\n' (or emits one pathologically large line) drives
// the bridge process to OOM. readBoundedLine caps a single frame.

import (
	"bufio"
	"errors"
)

// maxMCPFrameBytes is the hard cap on a single newline-delimited frame
// (and on an SSE event's accumulated data) read from an MCP server.
// 16 MiB is generous for legitimate JSON-RPC payloads while bounding the
// blast radius of a hostile or buggy server. It is a var (not a const)
// only so tests can lower it to drive the bound cheaply; each read loop
// captures it once at start (before its goroutine reads it), so the
// override is race-free for the set-before-construct test pattern.
var maxMCPFrameBytes = 16 << 20

// errMCPFrameTooLarge is returned when a server's frame exceeds
// maxMCPFrameBytes; the caller tears the transport down rather than
// letting the bridge die under memory pressure.
var errMCPFrameTooLarge = errors.New("mcpbridge: mcp server frame exceeds max size")

// readBoundedLine reads one newline-delimited frame from r, bounding the
// total to max bytes. It reads in buffer-sized chunks via ReadSlice
// (which returns bufio.ErrBufferFull when a line is longer than the
// reader's buffer), copying each chunk out before the next read so the
// returned slice is stable. Once the accumulated frame would exceed max
// it returns errMCPFrameTooLarge instead of allocating further. A
// trailing chunk with io.EOF (stream ended mid-line) is returned with
// that error, matching the prior ReadBytes/ReadString contract.
func readBoundedLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, errMCPFrameTooLarge
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return buf, err
	}
}
