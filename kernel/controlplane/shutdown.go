// SPDX-License-Identifier: MIT

package controlplane

// Graceful shutdown handler. Reaches the same exit path as SIGTERM
// but from any host with a valid control-plane token — the gap is
// scripted / CI workflows that need to stop the daemon without a
// shell on the host. Authorized via the same token every other
// command uses, so a leaked token is still the operator's blast
// radius to manage.

import (
	"time"
)

// shutdownAckGraceDelay is how long a shutdown waits between
// writing the OK response and signaling the daemon to exit. The
// delay exists so the client's blocking read on the response can
// complete before the kernel tears the TCP connection down on
// process exit. 50ms is generous on localhost (sub-millisecond
// RTT) but trivial vs the cost of a stuck client.
const shutdownAckGraceDelay = 50 * time.Millisecond

// scheduleShutdown signals the daemon to exit after the grace delay, so the
// acknowledgement reaches the client first. signalShutdown is idempotent, so
// concurrent shutdown requests resolve to one exit.
func (s *Server) scheduleShutdown() {
	go func() {
		time.Sleep(shutdownAckGraceDelay)
		s.signalShutdown()
	}()
}
