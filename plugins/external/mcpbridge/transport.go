// SPDX-License-Identifier: MIT

package main

// Transport abstraction (M1.MCP-SSE). MCP supports two transports
// in current and historical specs:
//
//   - **stdio**:        single child process; line-delimited JSON
//                       over stdin/stdout. Existing impl
//                       (stdio_transport.go).
//   - **HTTP + SSE**:   POST JSON-RPC to a session-scoped URL;
//                       receive responses + notifications via a
//                       long-lived `text/event-stream` GET.
//                       Implementation in sse_transport.go.
//
// The protocol-level mcpClient (call / notify / handshake /
// listTools / callTool / listResources / readResource / dead-state
// tracking) is transport-agnostic. Transports take a deliver
// callback at construction time and feed it parsed jsonrpc
// responses + raw notification lines + an eventual death error.
//
// Why callbacks instead of a `nextEvent()` channel: each transport
// already has its own read goroutine (read-stdout-line, read-sse-
// event). A channel would just be a marshalling layer between
// "I have a frame" and "I have a frame the client cares about" —
// callback delivery keeps the wire-decoded jsonrpcResp + notif
// shape internal to each transport and stops the mcpClient from
// caring whether the bytes came from a pipe or a socket.

import "context"

// transport is the wire layer below the JSON-RPC + MCP semantics.
// One transport instance corresponds to one open MCP session.
type transport interface {
	// send writes one JSON-RPC frame (request or notification).
	// Returns an error if the transport has died or the underlying
	// write failed. Concurrent sends are serialized inside the
	// transport — callers do not need to lock.
	send(req jsonrpcReq) error

	// close terminates the transport. Idempotent. Implementations
	// are responsible for cleaning up child processes, sockets,
	// or in-flight HTTP requests.
	close()
}

// transportDeliver is the contract from transport → mcpClient.
// Each transport runs its own read goroutine and pushes parsed
// events to the mcpClient via this interface as they arrive.
// All methods may be called from arbitrary goroutines; the
// mcpClient is internally synchronized.
type transportDeliver interface {
	// onResponse is invoked for each id-bearing JSON-RPC response.
	// The mcpClient routes it to the matching pending channel.
	onResponse(*jsonrpcResp)

	// onNotification is invoked for each id-less message (server
	// notifications). The raw bytes are passed so the existing
	// handleNotification dispatcher (which decodes Method+Params)
	// can stay unchanged.
	onNotification(raw []byte)

	// onTransportDead is invoked exactly once when the transport's
	// read goroutine exits — either because the peer closed the
	// connection, the child died, or close() was called. The
	// mcpClient flips itself dead and unblocks every pending caller.
	onTransportDead(error)
}

// transportFactory is what newMCPClient consumes — a function that
// builds a transport given a deliver target. Lets us inject either
// stdio or SSE without the protocol layer caring.
type transportFactory func(ctx context.Context, deliver transportDeliver) (transport, error)
