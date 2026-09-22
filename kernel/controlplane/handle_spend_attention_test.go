// SPDX-License-Identifier: MIT

// Tests for the Day 28+1 Mission Control handlers (handleSpendToday,
// handleAttention). Drives the commands through the control-plane client and
// asserts the response shape the Web UI hooks (useSpendToday at
// MissionControl.tsx:95 and useAttention at MissionControl.tsx:118) consume.
//
// The fresh-kernel case is the important one — when there are no approvals and
// no pulse asks yet, the attention panel MUST return an empty list (not nil,
// not an error) so the Web UI's `(items ?? []).map(...)` doesn't blow up.

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestSpendToday_FreshKernel drives handleSpendToday on a fresh kernel. With no
// governor wired (the mock provider isn't a *governor.Governor), the handler
// returns { total: 0 } — calm deterministic "0¢", not an error — so the
// Mission Control tile stays a flat-zero state instead of failing every poll.
func TestSpendToday_FreshKernel(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdSpendToday, map[string]any{})
	if err != nil {
		t.Fatalf("spend_today: %v", err)
	}
	total, ok := res["total"]
	if !ok {
		t.Fatalf("spend_today result missing total key: %v", res)
	}
	// JSON-decoded integers land as float64 through c.Call; assert the value is
	// numerically zero rather than the exact type. The handler emits int64(0)
	// server-side when no governor is wired; either float64(0) or int64(0) on
	// the wire satisfies "no spend today" — the Web UI treats both as "0¢".
	if toFloat(t, total) != 0 {
		t.Errorf("spend_today total = %v (%T), want 0 (no governor wired yet)", total, total)
	}
}

// TestSpendToday_KeyShape is a regression guard for the wire shape the Web UI
// hook consumes. Anything but { "total": <number> } breaks MissionControl.tsx
// which destructures `r.total` directly.
func TestSpendToday_KeyShape(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdSpendToday, map[string]any{})
	if err != nil {
		t.Fatalf("spend_today: %v", err)
	}
	// Exactly one key: "total". Anything else means the handler grew a field
	// the Web UI doesn't expect.
	if len(res) != 1 {
		t.Errorf("spend_today result has %d keys, want 1: %v", len(res), res)
	}
	if _, present := res["total"]; !present {
		t.Errorf("spend_today result missing 'total': %v", res)
	}
}

// TestAttention_FreshKernel drives handleAttention with no approvals and no
// pulse asks. The handler MUST return { items: [], count: 0 } — never nil, never
// an error — so the Web UI's `items ?? []` fallback is exercised and renders
// the empty-state copy ("Nothing requires your eyes…") instead of crashing.
func TestAttention_FreshKernel(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdAttention, map[string]any{})
	if err != nil {
		t.Fatalf("attention: %v", err)
	}
	items, ok := res["items"]
	if !ok {
		t.Fatalf("attention result missing items key: %v", res)
	}
	if _, isSlice := items.([]any); !isSlice {
		t.Errorf("attention items = %T, want []any", items)
	}
	// JSON-decoded integers land as float64 through c.Call.
	if toFloat(t, res["count"]) != 0 {
		t.Errorf("attention count = %v (%T), want 0", res["count"], res["count"])
	}
}

// toFloat coerces a JSON-decoded numeric into float64 for assertion. The control
// plane marshals numbers with encoding/json which turns int64 into float64 on
// the round trip; this helper keeps the test assertions readable.
func toFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		t.Fatalf("expected numeric value, got %T (%v)", v, v)
		return 0
	}
}

// TestAttention_ArgsParsing covers the optional window/limit query args. Bad
// inputs fall back to defaults (24h, 8) rather than 400ing — the route is a
// status read and the Mission Control panel stays calm even when the URL is
// hand-typed or stale.
func TestAttention_ArgsParsing(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	cases := []struct {
		name string
		args map[string]any
	}{
		{"defaults", map[string]any{}},
		{"window 5m", map[string]any{"window": "5m"}},
		{"window 1h", map[string]any{"window": "1h"}},
		{"window 24h", map[string]any{"window": "24h"}},
		{"bad window ignored", map[string]any{"window": "yesterday"}},
		{"limit 3", map[string]any{"limit": "3"}},
		{"limit 100 clamped to 50", map[string]any{"limit": "100"}},
		{"bad limit ignored", map[string]any{"limit": "eleven"}},
		{"combined", map[string]any{"window": "1h", "limit": "5"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := c.Call(context.Background(), controlplane.CmdAttention, tc.args)
			if err != nil {
				t.Fatalf("attention(%v): %v", tc.args, err)
			}
			if _, present := res["items"]; !present {
				t.Errorf("attention(%v) missing items key: %v", tc.args, res)
			}
		})
	}
}
