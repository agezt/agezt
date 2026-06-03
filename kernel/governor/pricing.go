// SPDX-License-Identifier: MIT

package governor

import (
	"math"
	"math/bits"
	"strings"
	"sync/atomic"

	"github.com/agezt/agezt/kernel/catalog"
)

// Pricing tables. All values in USD-microcents per million tokens
// (DECISIONS C1). 1 USD = 100 cents = 10^9 microcents, so
// "$3 per MTok" → 3 * 100 * 10_000_000 = 3_000_000_000 microcents/MTok.
//
// **Lookup order (M1.f):**
//
//   1. Live catalog (`SetCatalog` was called) — match by exact model ID
//      across every provider; if found, use Cost from catalog.
//   2. Fallback hardcoded table below — covers the case where the
//      operator hasn't synced yet (`agt catalog sync`) and offline
//      tests. Lives only for that bootstrap window; once the catalog
//      is populated it wins on every lookup.
//   3. Unknown model → 0 (free). Recorded in the budget.consumed event
//      so the operator can see "model X had no price entry".
//
// The catalog pointer is read via atomic.Pointer to keep the hot path
// (every Complete) lock-free. Swapping the catalog (post-sync) just
// stores a new pointer.

// modelPrice carries per-MTok prices for one model in microcents.
type modelPrice struct {
	InputMicrocentsPerMTok  int64
	OutputMicrocentsPerMTok int64
}

// liveCatalog is the swap-in catalog source consulted before the
// fallback table. Nil → only the fallback table is consulted (M1.b/c/d/e
// behaviour, preserved for offline tests).
var liveCatalog atomic.Pointer[catalog.Catalog]

// SetCatalog installs a live catalog as the primary pricing source.
// Called by runtime.Open after loading the on-disk catalog, and again
// by the control plane after `agt catalog sync`. Pass nil to revert
// to the hardcoded fallback table only.
func SetCatalog(c *catalog.Catalog) {
	liveCatalog.Store(c)
}

// modelPriceTable is the bootstrap fallback. Used only when the live
// catalog is absent OR doesn't contain the requested model. Numbers
// are list prices at the project's knowledge cutoff and will drift —
// run `agt catalog sync` to override.
var modelPriceTable = map[string]modelPrice{
	// Anthropic Claude (USD list per MTok, ×10^9 → microcents).
	"claude-opus-4-7":           {1_500_000_000, 7_500_000_000},
	"claude-opus-4-7[1m]":       {1_500_000_000, 7_500_000_000},
	"claude-opus-4-6":           {1_500_000_000, 7_500_000_000},
	"claude-sonnet-4-6":         {300_000_000, 1_500_000_000},
	"claude-sonnet-4-5":         {300_000_000, 1_500_000_000},
	"claude-haiku-4-5":          {80_000_000, 400_000_000},
	"claude-haiku-4-5-20251001": {80_000_000, 400_000_000},

	// Ollama / local models — always free.
	"llama3.2": {0, 0},
	"llama3.1": {0, 0},
	"qwen2.5":  {0, 0},
	"mistral":  {0, 0},
	"phi3":     {0, 0},
	"phi4":     {0, 0},
	"gemma2":   {0, 0},

	// Mock provider — free.
	"mock": {0, 0},
}

// priceFor looks up a model's price.
//
// Order: live catalog (exact match across providers) → fallback table
// (exact, then case-insensitive prefix) → zero.
//
// Unknown models cost nothing so we never block on a missing-price
// entry — but the model name lands in budget.consumed so the operator
// can see the gap.
func priceFor(model string) modelPrice {
	if model == "" {
		return modelPrice{}
	}
	if c := liveCatalog.Load(); c != nil {
		if _, m := c.FindModel(model); m != nil && m.Cost != nil {
			return modelPrice{
				InputMicrocentsPerMTok:  m.Cost.InputMicrocentsPerMTok(),
				OutputMicrocentsPerMTok: m.Cost.OutputMicrocentsPerMTok(),
			}
		}
	}
	if p, ok := modelPriceTable[model]; ok {
		return p
	}
	lower := strings.ToLower(model)
	for k, v := range modelPriceTable {
		if strings.HasPrefix(lower, strings.ToLower(k)) {
			return v
		}
	}
	return modelPrice{}
}

// CostMicrocents is the exported front-door (M1.oo) for the
// package's internal pricing math. Lets cross-package callers
// (notably the planner's cost-estimate path) project spend
// without duplicating the price table.
//
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
