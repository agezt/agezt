// SPDX-License-Identifier: MIT

package restapi

import (
	"net"
	"net/http"

	"github.com/agezt/agezt/kernel/streamlimit"
)

// maxSSEPerClient bounds concurrent Server-Sent-Events streams from one client
// IP on the REST surface (V-009) — the long-lived mailbox /watch stream is
// otherwise uncapped, so a token holder or buggy client could exhaust file
// descriptors/goroutines by opening watchers in a loop. Generous enough never to
// trip for legitimate SDK use; a resource guardrail like the body caps and
// slow-loris timeouts already in place.
const maxSSEPerClient = 64

var sseLimiter = streamlimit.New(maxSSEPerClient)

// sseGate reserves an SSE slot for the request's client IP. Returns (release,
// true) when under the cap; writes 429 + Retry-After and returns (noop, false)
// when over it, so the handler must return without opening the stream.
func (s *Server) sseGate(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	release, ok = sseLimiter.Acquire(streamClientKey(r))
	if !ok {
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusTooManyRequests, "too_many_streams", "too many concurrent streams from this client")
	}
	return release, ok
}

func streamClientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
