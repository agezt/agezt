// SPDX-License-Identifier: MIT

package controlplane

// M195: the dry-run warns when strict pricing would refuse the run (model
// has no known price), so the operator learns it before submitting.

import "testing"

func TestBuildRunPlan_StrictPricingWarning(t *testing.T) {
	// Strict on + unpriced (no known price) model → warn "would be REFUSED".
	p := buildRunPlan(runPlanInput{Model: "mystery-model", StrictPricing: true, ModelHasPrice: false})
	if !hasWarningContaining(p, "would be REFUSED") {
		t.Errorf("strict + unpriced should warn of refusal; got %v", warningsOf(p))
	}

	// Strict on + known-priced (or known-free) model → no refusal warning.
	p = buildRunPlan(runPlanInput{Model: "claude-sonnet-4-6", StrictPricing: true, ModelHasPrice: true})
	if hasWarningContaining(p, "would be REFUSED") {
		t.Errorf("strict + priced model must not warn of refusal; got %v", warningsOf(p))
	}

	// Strict OFF + unpriced model → no refusal warning (it's allowed, charged $0).
	p = buildRunPlan(runPlanInput{Model: "mystery-model", StrictPricing: false, ModelHasPrice: false})
	if hasWarningContaining(p, "would be REFUSED") {
		t.Errorf("strict off must not warn of refusal; got %v", warningsOf(p))
	}

	// Empty model (provider default) → not gated, no warning.
	p = buildRunPlan(runPlanInput{Model: "", StrictPricing: true, ModelHasPrice: false})
	if hasWarningContaining(p, "would be REFUSED") {
		t.Errorf("empty model must not warn of refusal; got %v", warningsOf(p))
	}
}
