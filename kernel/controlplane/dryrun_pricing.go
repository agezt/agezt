// SPDX-License-Identifier: MIT
//
// kernel/controlplane dry-run pricing helpers (modelPriced, strictPricingPlan, runPlanInput type).
// Extracted from dryrun.go during Day 211 god-file refactor (#100).
// Public API unchanged.
package controlplane

import (
	"time"

	"github.com/agezt/agezt/kernel/governor"
)

// modelPriced reports whether the Governor can price the model — i.e. whether a
// per-run cost cap (--max-cost) could ever trip for it. This is the AUTHORITATIVE
// check (catalog price → fallback table → unpriced fallback rate), unlike a
// catalog-only `m.Cost != nil` test which misses fallback-table-priced models.
// Only a model that prices to $0 can never exceed a positive cap, and since
// BIZ-001 that means a KNOWN-free/local model: an unrecognised name now bills at
// the unpriced fallback rate, so a cap does bind for it. Probed with 1 MTok
// in+out so any non-zero per-MTok rate registers.
func modelPriced(model string) bool {
	return governor.CostMicrocents(model, 1_000_000, 1_000_000) > 0
}

// strictPricingPlan derives the dry-run's strict-pricing inputs from the
// kernel's provider (M195): whether the daemon refuses unpriced models, and
// whether the effective model has a KNOWN price (incl. known-free). Kept here
// so server.go's handleRun needn't import governor. provider is k.Provider()
// (an agent.Provider); a non-governor provider (test rig) reports not-strict.
func strictPricingPlan(provider any, model string) (strict, hasPrice bool) {
	hasPrice = governor.ModelIsPriced(model)
	if gov, ok := provider.(*governor.Governor); ok {
		strict = gov.StrictPricingEnabled()
	}
	return
}

// runPlanInput carries the already-resolved primitives for a run, so buildRunPlan
// can stay pure (no kernel/catalog handles) and table-testable. handleRun fills it
// from the request args + kernel accessors when `dry_run` is set.
type runPlanInput struct {
	Intent           string
	Tenant           string // "" = primary kernel
	Model            string // effective model this run would use
	ModelOverridden  bool   // true if a per-run --model was given
	ModelKnown       bool   // catalog knows the effective model
	SupportsVision   bool   // catalog cap (meaningful only when ModelKnown)
	SupportsTools    bool   // catalog tool_call cap (meaningful only when ModelKnown)
	ContextLimit     int    // model's context window in tokens (0 = unknown)
	SystemSet        bool   // a daemon-default system prompt is configured
	SystemOverride   bool   // a per-run --system was given
	Timeout          string // per-run timeout (raw, validated); "" if none
	DaemonTimeout    time.Duration
	AllToolNames     []string // the kernel's full toolset (names)
	AllowSet         bool     // a "tools" arg was present (restriction in effect)
	Allow            []string // requested tool names (the allow-list)
	MaxCostMC        int64    // per-run cost cap in microcents (0 = none)
	ExecutionProfile string   // per-run execution profile id; "" = tool defaults
	WardenProfile    string   // requested warden profile when ExecutionProfile is set
	RemotePeer       string   // requested remote-agezt peer, if pinned
	ModelPriced      bool     // model prices to > $0 (a cap could trip) — catalog → fallback
	StrictPricing    bool     // daemon refuses unpriced models (AGEZT_PRICING_STRICT)
	ModelHasPrice    bool     // model has a KNOWN price entry (incl. known-free); != ModelPriced
}

// buildRunPlan resolves what a run WOULD do — effective model (and its catalog
// capabilities), the source of the system prompt, the effective wall-clock
// timeout, and the exact tool set the agent loop would see after the per-run
// filter — without executing anything or spending tokens. It is the payload of
// `agt run --dry-run`. Pure: deterministic in its input, no side effects.
