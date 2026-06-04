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
	// CacheReadMicrocentsPerMTok is the price of a prompt-cached input token
	// (0 when the model has no separate cache price → such tokens are billed at
	// the full input rate). Only the fallback table's pay-models leave it 0; the
	// live catalog populates it from cost.cache_read where present.
	CacheReadMicrocentsPerMTok int64
	// CacheWriteMicrocentsPerMTok is the price of a token written into the cache
	// (0 → billed at the input rate). Populated from cost.cache_write.
	CacheWriteMicrocentsPerMTok int64
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
// Fallback entries carry only input/output rates (cache-read left 0 → cached
// tokens bill at the input rate); the live catalog supplies cache-read prices.
var modelPriceTable = map[string]modelPrice{
	// Anthropic Claude (USD list per MTok, ×10^9 → microcents). Cache-read is
	// Anthropic's 0.1× input, cache-write 1.25× input list price (M289/M291).
	"claude-opus-4-7":           {InputMicrocentsPerMTok: 1_500_000_000, OutputMicrocentsPerMTok: 7_500_000_000, CacheReadMicrocentsPerMTok: 150_000_000, CacheWriteMicrocentsPerMTok: 1_875_000_000},
	"claude-opus-4-7[1m]":       {InputMicrocentsPerMTok: 1_500_000_000, OutputMicrocentsPerMTok: 7_500_000_000, CacheReadMicrocentsPerMTok: 150_000_000, CacheWriteMicrocentsPerMTok: 1_875_000_000},
	"claude-opus-4-6":           {InputMicrocentsPerMTok: 1_500_000_000, OutputMicrocentsPerMTok: 7_500_000_000, CacheReadMicrocentsPerMTok: 150_000_000, CacheWriteMicrocentsPerMTok: 1_875_000_000},
	"claude-sonnet-4-6":         {InputMicrocentsPerMTok: 300_000_000, OutputMicrocentsPerMTok: 1_500_000_000, CacheReadMicrocentsPerMTok: 30_000_000, CacheWriteMicrocentsPerMTok: 375_000_000},
	"claude-sonnet-4-5":         {InputMicrocentsPerMTok: 300_000_000, OutputMicrocentsPerMTok: 1_500_000_000, CacheReadMicrocentsPerMTok: 30_000_000, CacheWriteMicrocentsPerMTok: 375_000_000},
	"claude-haiku-4-5":          {InputMicrocentsPerMTok: 80_000_000, OutputMicrocentsPerMTok: 400_000_000, CacheReadMicrocentsPerMTok: 8_000_000, CacheWriteMicrocentsPerMTok: 100_000_000},
	"claude-haiku-4-5-20251001": {InputMicrocentsPerMTok: 80_000_000, OutputMicrocentsPerMTok: 400_000_000, CacheReadMicrocentsPerMTok: 8_000_000, CacheWriteMicrocentsPerMTok: 100_000_000},

	// Ollama / local models — always free.
	"llama3.2": {},
	"llama3.1": {},
	"qwen2.5":  {},
	"mistral":  {},
	"phi3":     {},
	"phi4":     {},
	"gemma2":   {},

	// Mock provider — free.
	"mock": {},
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
	p, _ := priceForOk(model)
	return p
}

// modelIsPriced reports whether the model has a KNOWN price entry — in the
// live catalog or the fallback table (exact or longest-prefix). A model in
// the table at price {0,0} (a local/free model) counts as PRICED; only a
// genuinely unknown model is unpriced. Strict-pricing mode (M193) uses this
// to refuse unpriced models that would otherwise be charged $0 and bypass
// the budget.
func modelIsPriced(model string) bool {
	_, ok := priceForOk(model)
	return ok
}

// priceForOk is priceFor plus a found flag, so callers can distinguish a
// known-free model ({0,0}, found) from an unknown one ({0,0}, not found).
func priceForOk(model string) (modelPrice, bool) {
	if model == "" {
		return modelPrice{}, false
	}
	if c := liveCatalog.Load(); c != nil {
		if _, m := c.FindModel(model); m != nil && m.Cost != nil {
			return modelPrice{
				InputMicrocentsPerMTok:      m.Cost.InputMicrocentsPerMTok(),
				OutputMicrocentsPerMTok:     m.Cost.OutputMicrocentsPerMTok(),
				CacheReadMicrocentsPerMTok:  m.Cost.CacheReadMicrocentsPerMTok(),
				CacheWriteMicrocentsPerMTok: m.Cost.CacheWriteMicrocentsPerMTok(),
			}, true
		}
	}
	if p, ok := modelPriceTable[model]; ok {
		return p, true
	}
	// Prefix fallback for versioned suffixes (e.g. a new dated snapshot
	// `claude-haiku-4-5-20260101` pricing like its base `claude-haiku-4-5`).
	// Pick the LONGEST matching key (M192), not the first one Go's
	// randomized map iteration happens to hit: returning the first match
	// made the price of an overlapping name nondeterministic across boots
	// — unacceptable for money math — and could bind a model to a less
	// specific (cheaper) entry than the best available. Longest-prefix is
	// deterministic (no two distinct keys of equal length can both prefix
	// the same string) and always prefers the most specific price.
	lower := strings.ToLower(model)
	bestLen := -1
	var best modelPrice
	for k, v := range modelPriceTable {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lower, lk) && len(lk) > bestLen {
			bestLen = len(lk)
			best = v
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return modelPrice{}, false
}

// ModelIsPriced is the exported front-door for modelIsPriced (M195): it
// reports whether the governor has a KNOWN price for the model — in the
// catalog or the fallback table. A known-FREE model (local/mock, priced 0)
// is priced (true); only a genuinely unknown model is not. Distinct from
// `CostMicrocents(...) > 0`, which is false for known-free models too.
// Used by the dry-run to predict whether StrictPricing would refuse a run.
func ModelIsPriced(model string) bool {
	return modelIsPriced(model)
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
