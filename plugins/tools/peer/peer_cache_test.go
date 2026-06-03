// SPDX-License-Identifier: MIT

package peer

import (
	"testing"
	"time"
)

// TestAutoRoute_CachesDiscovery proves repeated auto-routes within the TTL reuse the
// discovery result instead of re-probing every peer, and that the cache expires.
func TestAutoRoute_CachesDiscovery(t *testing.T) {
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	clk := base
	calls := 0

	tool := &Tool{
		Peers:    twoPeers(),
		CacheTTL: 30 * time.Second,
		now:      func() time.Time { return clk },
		post:     fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, nil, nil, nil),
		// alpha doesn't serve opus, bravo does — both peers are consulted each miss.
		list: fakeList(map[string][]string{"alpha": {"haiku"}, "bravo": {"opus"}}, &calls),
	}

	// First auto-route: both peers probed once (alpha miss, bravo match).
	if _, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"}); isErr {
		t.Fatal("invoke 1 errored")
	}
	if calls != 2 {
		t.Fatalf("after first route want 2 discovery calls, got %d", calls)
	}

	// Second auto-route within the TTL: both peers served from cache, no new calls.
	if _, isErr := invoke(t, tool, map[string]string{"task": "y", "model": "opus"}); isErr {
		t.Fatal("invoke 2 errored")
	}
	if calls != 2 {
		t.Errorf("within TTL want no new discovery calls, got %d total", calls)
	}

	// Advance past the TTL: the next route re-probes both peers.
	clk = base.Add(31 * time.Second)
	if _, isErr := invoke(t, tool, map[string]string{"task": "z", "model": "opus"}); isErr {
		t.Fatal("invoke 3 errored")
	}
	if calls != 4 {
		t.Errorf("after TTL expiry want re-probe (4 total), got %d", calls)
	}
}

// TestAutoRoute_DoesNotCacheErrors proves a discovery error is not cached: a peer
// that fails discovery then recovers is re-probed on the next route.
func TestAutoRoute_DoesNotCacheErrors(t *testing.T) {
	clk := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	calls := 0

	// alpha is unreachable; bravo serves opus. alpha's error must not be cached.
	tool := &Tool{
		Peers:    twoPeers(),
		CacheTTL: time.Hour,
		now:      func() time.Time { return clk },
		post:     fakePost(200, `{"status":"completed","answer":"ok","correlation_id":"c"}`, nil, nil, nil),
		list:     fakeList(map[string][]string{"bravo": {"opus"}}, &calls, "alpha"),
	}

	for i := 0; i < 2; i++ {
		if _, isErr := invoke(t, tool, map[string]string{"task": "x", "model": "opus"}); isErr {
			t.Fatalf("invoke %d errored", i)
		}
	}
	// bravo is cached after the first route (1 call); alpha errors both times (not
	// cached) → 2 alpha calls. Total = 1 (bravo) + 2 (alpha) = 3.
	if calls != 3 {
		t.Errorf("want bravo cached + alpha re-probed (3 calls), got %d", calls)
	}
}
