// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"time"
)

// disconnectSignal reports the client connection closing.
//
// The run operation uses it to cancel its run when the operator's client goes
// away (Ctrl-C / killed), instead of letting the run keep spending into a
// closed connection. The client sends nothing after its request, so a read
// unblocks only when the connection closes.
//
// The read deadline is cleared first: handleConn arms a 10-minute deadline
// per request, and a long run must not be mistaken for a disconnect when that
// deadline elapses. When the run finishes normally, handleConn's deferred
// conn.Close() ends the read, and the cancellation is a harmless no-op (the
// run is already gone).
func disconnectSignal(conn net.Conn) <-chan struct{} {
	signal := make(chan struct{})
	go func() {
		defer close(signal)
		if conn == nil {
			return
		}
		_ = conn.SetReadDeadline(time.Time{})
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
	}()
	return signal
}
