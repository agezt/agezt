// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// rateLimitedGovernor builds a governor that admits at most `perMin` calls per
// minute, so the next call is throttled (rate.limited journaled).
func rateLimitedGovernor(t *testing.T, perMin int) *governor.Governor {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name:     "mock",
		Provider: mock.New(mock.FinalText("ok")),
		AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	g, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: 1_000_000_000,
		RateLimitPerMin:        perMin,
		Now:                    func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	// The daemon has no default run model, so a bare CmdRun carries an empty model
	// and would be refused with ErrNoModelConfigured before the rate limiter ever
	// runs. Give the governor a default fallback chain (the production way to
	// resolve a model) so the throttling path is exercised.
	g.SetFallbackChains(map[string][]string{"d": {"m"}}, "d")
	return g
}

func TestRateLimitLogAndStats(t *testing.T) {
	gov := rateLimitedGovernor(t, 1) // admit 1 call/min, throttle the rest
	k, _, c, _ := startPair(t, gov)
	gov.SetBus(k.Bus()) // route rate.limited events into the kernel journal (as the daemon does)

	// First run admitted; subsequent runs in the same minute get throttled.
	for i := 0; i < 3; i++ {
		_, _ = c.Stream(context.Background(), controlplane.CmdRun, map[string]any{"intent": "x"}, nil)
	}

	stats, err := c.Call(context.Background(), controlplane.CmdRateLimitStats, nil)
	if err != nil {
		t.Fatalf("ratelimit stats: %v", err)
	}
	throttled := 0
	if f, ok := stats["throttled"].(float64); ok {
		throttled = int(f)
	}
	if throttled < 1 {
		t.Fatalf("expected at least one throttle, got %d", throttled)
	}
	if lim, _ := stats["limit_per_min"].(float64); int(lim) != 1 {
		t.Errorf("limit_per_min = %v, want 1", stats["limit_per_min"])
	}

	logRes, err := c.Call(context.Background(), controlplane.CmdRateLimitLog, nil)
	if err != nil {
		t.Fatalf("ratelimit log: %v", err)
	}
	rows, _ := logRes["throttles"].([]any)
	if len(rows) < 1 {
		t.Fatalf("ratelimit log returned %d rows, want >=1", len(rows))
	}
}

func TestRateLimitStats_Empty(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	stats, err := c.Call(context.Background(), controlplane.CmdRateLimitStats, nil)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if th, _ := stats["throttled"].(float64); th != 0 {
		t.Errorf("throttled = %v on a fresh kernel, want 0", stats["throttled"])
	}
}
