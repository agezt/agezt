// SPDX-License-Identifier: MIT

package governor

import (
	"math"
	"testing"
)

// FuzzCostMicrocents hardens the cost math, which converts UNTRUSTED
// provider-reported token counts into microcents that drive billing and the daily
// spend ceiling. The load-bearing financial-control invariant: cost is NEVER
// negative. A negative cost (from an integer overflow that wraps, or unhandled
// negative token counts) would credit the ledger and effectively disable the
// ceiling — a spend-cap bypass. The math saturates to MaxInt64 on overflow
// (fail-closed); this fuzz proves that holds for any inputs, including negative,
// huge, and MaxInt token counts, and any model string (known or unknown).
func FuzzCostMicrocents(f *testing.F) {
	f.Add("claude-3-5-haiku-20241022", 100, 0, 0, 50)
	f.Add("unknown-model", 0, 0, 0, 0)
	f.Add("x", -1, -1, -1, -1)
	f.Add("y", math.MaxInt, math.MaxInt, math.MaxInt, math.MaxInt)
	f.Add("z", math.MinInt, 0, 0, math.MaxInt)

	f.Fuzz(func(t *testing.T, model string, in, cached, write, out int) {
		if c := costMicrocents(model, in, out); c < 0 {
			t.Errorf("costMicrocents(%q,%d,%d) = %d — a negative cost would credit the ledger and disable the ceiling", model, in, out, c)
		}
		if c := costMicrocentsCached(model, in, cached, write, out); c < 0 {
			t.Errorf("costMicrocentsCached(%q,%d,%d,%d,%d) = %d — negative cost", model, in, cached, write, out, c)
		}
	})
}
