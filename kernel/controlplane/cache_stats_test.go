// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestCacheStats_Aggregates — `cache_stats` folds budget.consumed into cached /
// written token sums and the savings vs the no-cache baseline (M293).
func TestCacheStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	// One cache-heavy call on a priced model (claude-sonnet-4-6: input 300M,
	// cache-read 30M, cache-write 375M, output 1500M per MTok). Cost below is the
	// cache-aware figure (= governor.costMicrocentsCached for these counts).
	k.Bus().Publish(event.Spec{
		Subject: "governor.budget", Kind: event.KindBudgetConsumed, Actor: "governor",
		Payload: map[string]any{
			"model":                    "claude-sonnet-4-6",
			"input_tokens":             10000,
			"cached_input_tokens":      9000,
			"cache_write_input_tokens": 500,
			"output_tokens":            200,
			"cost_microcents":          907500,
		},
	})

	res, err := c.Call(context.Background(), controlplane.CmdCacheStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := toI(res["cached_input_tokens"]); got != 9000 {
		t.Errorf("cached_input_tokens = %d want 9000", got)
	}
	if got := toI(res["cache_write_input_tokens"]); got != 500 {
		t.Errorf("cache_write_input_tokens = %d want 500", got)
	}
	// Baseline (no cache) = (10000*300M + 200*1500M)/1e6 = 3_300_000 mc.
	// Saved = 3_300_000 − 907_500 = 2_392_500 mc.
	if got := toI(res["saved_microcents"]); got != 2_392_500 {
		t.Errorf("saved_microcents = %d want 2392500", got)
	}
	if got := toI(res["calls"]); got != 1 {
		t.Errorf("calls = %d want 1", got)
	}
}

// toI coerces a JSON number (decoded as float64 over the wire) to int64.
func toI(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
