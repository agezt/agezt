// SPDX-License-Identifier: MIT

package runs

import (
	"github.com/agezt/agezt/kernel/governor"
)

// modelPriced reports whether the Governor can price the model — i.e. whether
// a per-run cost cap (--max-cost) could ever trip for it. This is the
// AUTHORITATIVE check (catalog price → fallback table → unpriced fallback
// rate), unlike a catalog-only "cost != nil" test which misses
// fallback-table-priced models. Only a model that prices to $0 can never
// exceed a positive cap, and since BIZ-001 that means a KNOWN-free/local
// model: an unrecognised name now bills at the unpriced fallback rate, so a
// cap does bind for it. Probed with 1 MTok in+out so any non-zero per-MTok
// rate registers.
func modelPriced(model string) bool {
	return governor.CostMicrocents(model, 1_000_000, 1_000_000) > 0
}

// strictPricingPlan derives the dry-run's strict-pricing inputs from the
// kernel's provider: whether the daemon refuses unpriced models, and whether
// the effective model has a KNOWN price (incl. known-free). provider is the
// kernel's provider; a non-governor provider (a test rig) reports not-strict.
func strictPricingPlan(provider any, model string) (strict, hasPrice bool) {
	hasPrice = governor.ModelIsPriced(model)
	if gov, ok := provider.(*governor.Governor); ok {
		strict = gov.StrictPricingEnabled()
	}
	return
}
