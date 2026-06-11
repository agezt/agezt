// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// TestResponseCache_HitSkipsProvider (M888): with ResponseCacheTTL set, an
// IDENTICAL second request is served from memory (one provider call total),
// a different request misses, and an expired entry re-fetches. Disabled
// (default) caches nothing.
func TestResponseCache_HitSkipsProvider(t *testing.T) {
	mkReq := func(text string) agent.CompletionRequest {
		return agent.CompletionRequest{
			Model:    "m",
			Messages: []agent.Message{{Role: agent.RoleUser, Content: text}},
		}
	}

	t.Run("enabled", func(t *testing.T) {
		b, _ := newBus(t)
		now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
		clock := func() time.Time { return now }
		prov := &fakeProvider{name: "p1", resp: okResp("m", 5, 5)}
		r := governor.NewRegistry()
		mustRegister(t, r, &governor.ProviderInfo{Name: "p1", Provider: prov, AuthMode: governor.AuthLocal})
		g, err := governor.New(governor.Config{Registry: r, Bus: b, Now: clock, ResponseCacheTTL: 5 * time.Minute})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		if _, err := g.Complete(context.Background(), mkReq("q1")); err != nil {
			t.Fatalf("Complete 1: %v", err)
		}
		resp, err := g.Complete(context.Background(), mkReq("q1")) // identical → cached
		if err != nil {
			t.Fatalf("Complete 2: %v", err)
		}
		if resp.Message.Content != "ok from m" {
			t.Errorf("cached content = %q", resp.Message.Content)
		}
		if got := prov.calls.Load(); got != 1 {
			t.Errorf("provider calls after identical repeat = %d, want 1 (second served from cache)", got)
		}

		if _, err := g.Complete(context.Background(), mkReq("q2")); err != nil { // different → miss
			t.Fatalf("Complete 3: %v", err)
		}
		if got := prov.calls.Load(); got != 2 {
			t.Errorf("provider calls after a different request = %d, want 2", got)
		}

		now = now.Add(6 * time.Minute) // q1's entry expires
		if _, err := g.Complete(context.Background(), mkReq("q1")); err != nil {
			t.Fatalf("Complete 4: %v", err)
		}
		if got := prov.calls.Load(); got != 3 {
			t.Errorf("provider calls after TTL expiry = %d, want 3 (stale entry re-fetched)", got)
		}
	})

	t.Run("disabled by default", func(t *testing.T) {
		b, _ := newBus(t)
		prov := &fakeProvider{name: "p1", resp: okResp("m", 5, 5)}
		r := governor.NewRegistry()
		mustRegister(t, r, &governor.ProviderInfo{Name: "p1", Provider: prov, AuthMode: governor.AuthLocal})
		g, err := governor.New(governor.Config{Registry: r, Bus: b})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		for range 2 {
			if _, err := g.Complete(context.Background(), mkReq("same")); err != nil {
				t.Fatalf("Complete: %v", err)
			}
		}
		if got := prov.calls.Load(); got != 2 {
			t.Errorf("provider calls = %d, want 2 — caching must be opt-in", got)
		}
	})
}
