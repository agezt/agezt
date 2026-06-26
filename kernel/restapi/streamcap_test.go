// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSSEGate_OverCapReturns429 verifies the V-009 stream cap: once a client IP
// holds maxSSEPerClient slots, the next stream is refused with 429 + Retry-After
// (not opened), while a different IP is unaffected. Acquired slots are released
// so the shared process limiter isn't polluted for other tests.
func TestSSEGate_OverCapReturns429(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "tok")

	const ip = "203.0.113.7"
	rels := make([]func(), 0, maxSSEPerClient)
	defer func() {
		for _, r := range rels {
			r()
		}
	}()
	for i := 0; i < maxSSEPerClient; i++ {
		r, ok := sseLimiter.Acquire(ip)
		if !ok {
			t.Fatalf("acquire %d unexpectedly refused below cap", i)
		}
		rels = append(rels, r)
	}

	// Same IP, now over cap → 429 and gate refuses.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mailbox/watch", nil)
	req.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	if _, ok := s.sseGate(rec, req); ok {
		t.Fatal("gate must refuse a client already at its cap")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on refusal")
	}

	// A different IP is unaffected.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/mailbox/watch", nil)
	req2.RemoteAddr = "198.51.100.4:40001"
	rec2 := httptest.NewRecorder()
	release, ok := s.sseGate(rec2, req2)
	if !ok {
		t.Fatal("a different client IP must not be capped")
	}
	release()
}
