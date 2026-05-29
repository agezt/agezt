// SPDX-License-Identifier: MIT

package controlplane

// Graceful shutdown handler. Reaches the same exit path as SIGTERM
// but from any host with a valid control-plane token — the gap is
// scripted / CI workflows that need to stop the daemon without a
// shell on the host. Authorized via the same token every other
// command uses, so a leaked token is still the operator's blast
// radius to manage.

import (
	"net"
	"time"
)

// shutdownAckGraceDelay is how long handleShutdown waits between
// writing the OK response and signaling the daemon to exit. The
// delay exists so the client's blocking read on the response can
// complete before the kernel tears the TCP connection down on
// process exit. 50ms is generous on localhost (sub-millisecond
// RTT) but trivial vs the cost of a stuck client.
const shutdownAckGraceDelay = 50 * time.Millisecond

func (s *Server) handleShutdown(conn net.Conn, req Request) {
	// Write the success response FIRST so the client gets a clean
	// confirmation before the daemon starts exiting.
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"ok": true},
	})
	// Schedule the actual shutdown async so this handler can return,
	// the conn close defers run, and the OS gets the response bytes
	// flushed before main() exits. signalShutdown is idempotent —
	// concurrent CmdShutdown requests resolve to one shutdown.
	go func() {
		time.Sleep(shutdownAckGraceDelay)
		s.signalShutdown()
	}()
}
