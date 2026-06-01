// SPDX-License-Identifier: MIT

package main

import "testing"

func TestEstimateCostMicrocents(t *testing.T) {
	// $3/Mtok in, $15/Mtok out → microcents: $1 = 1e9 mc, so $3/Mtok = 3e9 mc.
	inMc := int64(3_000_000_000)
	outMc := int64(15_000_000_000)
	// 1,000,000 input tokens = exactly $3.00 = 3e9 mc.
	if got := estimateCostMicrocents(inMc, outMc, 1_000_000, 0); got != 3_000_000_000 {
		t.Errorf("1M input: got %d, want 3e9", got)
	}
	// 200,000 output tokens = $15 * 0.2 = $3.00 = 3e9 mc.
	if got := estimateCostMicrocents(inMc, outMc, 0, 200_000); got != 3_000_000_000 {
		t.Errorf("200k output: got %d, want 3e9", got)
	}
	// Combined: 1M in + 200k out = $6.00.
	if got := estimateCostMicrocents(inMc, outMc, 1_000_000, 200_000); got != 6_000_000_000 {
		t.Errorf("combined: got %d, want 6e9", got)
	}
	// Zero tokens → zero cost.
	if got := estimateCostMicrocents(inMc, outMc, 0, 0); got != 0 {
		t.Errorf("zero: got %d, want 0", got)
	}
}

func TestFindModelCost(t *testing.T) {
	res := map[string]any{
		"providers": []any{
			map[string]any{"id": "anthropic", "models": []any{
				map[string]any{"id": "claude-sonnet-4-6", "cost_input_mc_per_mtok": float64(3_000_000_000)},
			}},
			map[string]any{"id": "openai", "models": []any{
				map[string]any{"id": "gpt-x"},
			}},
		},
	}
	entry, prov, found := findModelCost(res, "claude-sonnet-4-6")
	if !found || prov != "anthropic" {
		t.Fatalf("found=%v prov=%q, want true/anthropic", found, prov)
	}
	if mcFromAny(entry["cost_input_mc_per_mtok"]) != 3_000_000_000 {
		t.Errorf("entry cost wrong: %v", entry["cost_input_mc_per_mtok"])
	}
	if _, _, ok := findModelCost(res, "nonexistent"); ok {
		t.Errorf("nonexistent model should not be found")
	}
}

func TestCommaInt(t *testing.T) {
	cases := map[int64]string{0: "0", 5: "5", 100: "100", 1000: "1,000", 1000000: "1,000,000", 200000: "200,000"}
	for in, want := range cases {
		if got := commaInt(in); got != want {
			t.Errorf("commaInt(%d) = %q, want %q", in, got, want)
		}
	}
}
