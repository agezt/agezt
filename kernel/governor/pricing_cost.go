// SPDX-License-Identifier: MIT

// Package governor: cost computation + saturation-math helpers
// (CostMicrocents + costMicrocents + costMicrocentsCached +
// saturatingMul + saturatingAdd). The saturatingMul/saturatingAdd helpers
// protect against malicious or buggy token counts. Extracted from pricing.go
// during the Day-211 god-file split. Public API unchanged.
package governor


import (
	"math"
	"math/bits"
)
// Behaviour identical to the internal costMicrocents.
func CostMicrocents(model string, inputTokens, outputTokens int) int64 {
	return costMicrocents(model, inputTokens, outputTokens)
}

// costMicrocents returns the cost of input_tokens + output_tokens at
// the given model's prices, computed as integer microcents
// (DECISIONS C1: no float drift).
//
//	cost_microcents = (input_tokens * input_mc_per_MTok + output * out_mc_per_MTok) / 1_000_000
func costMicrocents(model string, inputTokens, outputTokens int) int64 {
	p := priceFor(model)
	// Token counts come from the (untrusted) provider usage response; a
	// buggy or hostile endpoint can report negative or absurd values.
	// Compute with saturation (M191): negatives are treated as 0 and any
	// int64 overflow saturates to MaxInt64 instead of wrapping. This is
	// fail-CLOSED — a nonsensical usage report yields a huge cost that
	// trips the budget gate, never a negative cost that would credit the
	// ledger and disable the daily ceiling.
	totalMicromicrocents := saturatingAdd(
		saturatingMul(inputTokens, p.InputMicrocentsPerMTok),
		saturatingMul(outputTokens, p.OutputMicrocentsPerMTok),
	)
	return totalMicromicrocents / 1_000_000
}

// costMicrocentsCached is the cache-aware billing path (M289/M291). cachedTokens
// (prompt-cache reads) and writeTokens (prompt-cache creations) are SUBSETS of
// inputTokens: reads bill at the cache-read rate, writes at the cache-write rate,
// and the fresh remainder at the input rate. Same saturating integer math as
// costMicrocents.
//
//	cost = fresh*input + cached*cache_read + write*cache_write + output*output
//	fresh = input - cached - write
//
// A subset with no separate price (CacheRead/CacheWrite == 0) bills at the full
// input rate — conservative (never under-bill an unknown cache rate). With
// cachedTokens == writeTokens == 0 the result is identical to costMicrocents.
func costMicrocentsCached(model string, inputTokens, cachedTokens, writeTokens, outputTokens int) int64 {
	p := priceFor(model)
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if writeTokens < 0 {
		writeTokens = 0
	}
	// cached + write are subsets of the prompt; clamp their sum to inputTokens
	// (a buggy/hostile report claiming more must not credit the ledger).
	if cachedTokens > inputTokens {
		cachedTokens = inputTokens
	}
	if cachedTokens+writeTokens > inputTokens {
		writeTokens = inputTokens - cachedTokens
	}
	cacheRate := p.CacheReadMicrocentsPerMTok
	if cacheRate <= 0 {
		cacheRate = p.InputMicrocentsPerMTok // no cache-read price → full input rate
	}
	writeRate := p.CacheWriteMicrocentsPerMTok
	if writeRate <= 0 {
		writeRate = p.InputMicrocentsPerMTok // no cache-write price → full input rate
	}
	fresh := inputTokens - cachedTokens - writeTokens
	totalMicromicrocents := saturatingAdd(
		saturatingAdd(
			saturatingMul(fresh, p.InputMicrocentsPerMTok),
			saturatingMul(cachedTokens, cacheRate),
		),
		saturatingAdd(
			saturatingMul(writeTokens, writeRate),
			saturatingMul(outputTokens, p.OutputMicrocentsPerMTok),
		),
	)
	return totalMicromicrocents / 1_000_000
}

// saturatingMul returns tokens * pricePerMTok clamped to [0, MaxInt64].
// Non-positive tokens or price yield 0; a product that exceeds int64
// saturates to MaxInt64 rather than wrapping negative.
func saturatingMul(tokens int, pricePerMTok int64) int64 {
	if tokens <= 0 || pricePerMTok <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(tokens), uint64(pricePerMTok))
	if hi != 0 || lo > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(lo)
}

// saturatingAdd returns a + b (both assumed >= 0) clamped to MaxInt64.
func saturatingAdd(a, b int64) int64 {
	sum := a + b
	if sum < a { // overflowed (a, b are non-negative)
		return math.MaxInt64
	}
	return sum
}
