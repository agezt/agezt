// SPDX-License-Identifier: MIT

package webui

import (
	"net"
	"net/http"

	"github.com/agezt/agezt/kernel/streamlimit"
)

// maxSSEPerClient bounds the number of concurrent Server-Sent-Events streams a
// single client IP may hold open against the console (V-009). The long-lived
// /events firehose otherwise has no per-client cap, so a leaked token or a buggy
// client could exhaust file descriptors and goroutines by opening streams in a
// loop. The bound is deliberately generous — a browser opens a handful of
// EventSource connections — so it never trips for legitimate use and only fences
// the pathological case. It is a resource guardrail like the existing body caps
// and slow-loris timeouts, not a feature gate.
const maxSSEPerClient = 64

// sseLimiter caps concurrent SSE streams per client key for the whole console
// process. One limiter for the daemon's single web server.
var sseLimiter = streamlimit.New(maxSSEPerClient)

// sseGate reserves an SSE slot for the request's client. On success it returns a
// release func (call it via defer when the stream ends) and true. When the
// client is already at its cap it writes 429 + Retry-After and returns false, so
// the handler must return without opening the stream.
func sseGate(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	release, ok = sseLimiter.Acquire(streamClientKey(r))
	if !ok {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "too many concurrent streams from this client", http.StatusTooManyRequests)
	}
	return release, ok
}

// streamClientKey identifies the client for stream-cap accounting: the remote
// IP (host part of RemoteAddr). Behind a reverse proxy all clients share the
// proxy's address, so the cap is effectively per-proxy there — acceptable for a
// loopback-default console; the cap exists to bound abuse, not to meter tenants.
func streamClientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
