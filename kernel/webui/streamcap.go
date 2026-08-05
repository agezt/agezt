// SPDX-License-Identifier: MIT

package webui

import (
	"net"
	"net/http"
)

// The per-client SSE stream cap (V-009) lives in kernel/httpserver now
// (httpserver.StartSSE / AcquireSSE) — one process-wide limiter shared by
// every HTTP surface instead of a private copy per package (LD-8).

// streamClientKey identifies the client for per-client rate accounting (the
// /hooks limiter): the remote IP (host part of RemoteAddr). Behind a reverse
// proxy all clients share the proxy's address, so the cap is effectively
// per-proxy there — acceptable for a loopback-default console; the cap exists
// to bound abuse, not to meter tenants.
func streamClientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
