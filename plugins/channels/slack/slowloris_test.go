// SPDX-License-Identifier: MIT

package slack

import "testing"

// TestNewHTTPServer_SlowLorisTimeouts: the inbound HTTP server must set
// ReadHeaderTimeout + ReadTimeout (bounding a slow-dripped body) and IdleTimeout, so a
// client can't hold a handler goroutine open indefinitely (M431).
func TestNewHTTPServer_SlowLorisTimeouts(t *testing.T) {
	c := New(Config{})
	srv := c.newHTTPServer()
	if srv.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout unset")
	}
	if srv.ReadTimeout == 0 {
		t.Error("ReadTimeout unset — a slow-dripped request body can pin the handler goroutine")
	}
	if srv.IdleTimeout == 0 {
		t.Error("IdleTimeout unset")
	}
}
