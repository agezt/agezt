// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"testing"
)

// TestGuardedHTTPServer_SlowLorisTimeouts: every HTTP surface (web UI, OpenAI API,
// REST) must be built with a ReadHeaderTimeout + IdleTimeout so a slow client cannot
// pin connections/goroutines indefinitely (M419) — but WriteTimeout must stay unset
// so long-lived SSE/streaming responses are not killed mid-flight.
func TestGuardedHTTPServer_SlowLorisTimeouts(t *testing.T) {
	srv := newGuardedHTTPServer(http.NewServeMux())
	if srv.ReadHeaderTimeout == 0 || srv.ReadHeaderTimeout != httpReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v (slow-loris guard)", srv.ReadHeaderTimeout, httpReadHeaderTimeout)
	}
	if srv.IdleTimeout == 0 || srv.IdleTimeout != httpIdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", srv.IdleTimeout, httpIdleTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (a WriteTimeout would kill SSE/streaming)", srv.WriteTimeout)
	}
}
