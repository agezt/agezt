// SPDX-License-Identifier: MIT

package controlplane

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/governor"
)

// smallContextThreshold mirrors catalog.Model.AgentWarnings: a context window
// below this is flagged as a risk for long agent runs with memory + tools.
const smallContextThreshold = 8192

// microcentsPerUSD is the kernel's spend unit: $1 = 1e9 USD-microcents (matches
// governor.DefaultDailyCeilingMicrocents and the CLI's --max-cost parser).
const microcentsPerUSD = 1_000_000_000

// formatMicrocentsUSD renders a microcent amount as a dollar string (e.g.
// 500_000_000 → "$0.50"). Used for the dry-run cost-cap line/advisory.
func formatMicrocentsUSD(mc int64) string {
	return fmt.Sprintf("$%.2f", float64(mc)/microcentsPerUSD)
}

// modelPriced reports whether the Governor can price the model — i.e. whether a
// per-run cost cap (--max-cost) could ever trip for it. This is the AUTHORITATIVE
// check (catalog price → fallback table), unlike a catalog-only `m.Cost != nil`
// test which misses fallback-table-priced or catalog-unknown models. A model that
// prices to $0 (unknown, or free/local) can never exceed a positive cap. Probed
// with 1 MTok in+out so any non-zero per-MTok rate registers.
func modelPriced(model string) bool {
	return governor.CostMicrocents(model, 1_000_000, 1_000_000) > 0
}

// runPlanInput carries the already-resolved primitives for a run, so buildRunPlan
// can stay pure (no kernel/catalog handles) and table-testable. handleRun fills it
// from the request args + kernel accessors when `dry_run` is set.
type runPlanInput struct {
	Intent          string
	Tenant          string // "" = primary kernel
	Model           string // effective model this run would use
	ModelOverridden bool   // true if a per-run --model was given
	ModelKnown      bool   // catalog knows the effective model
	SupportsVision  bool   // catalog cap (meaningful only when ModelKnown)
	SupportsTools   bool   // catalog tool_call cap (meaningful only when ModelKnown)
	ContextLimit    int    // model's context window in tokens (0 = unknown)
	SystemSet       bool   // a daemon-default system prompt is configured
	SystemOverride  bool   // a per-run --system was given
	Timeout         string // per-run timeout (raw, validated); "" if none
	DaemonTimeout   time.Duration
	AllToolNames    []string // the kernel's full toolset (names)
	AllowSet        bool     // a "tools" arg was present (restriction in effect)
	Allow           []string // requested tool names (the allow-list)
	MaxCostMC       int64    // per-run cost cap in microcents (0 = none)
	ModelPriced     bool     // catalog has a price for the effective model
}

// buildRunPlan resolves what a run WOULD do — effective model (and its catalog
// capabilities), the source of the system prompt, the effective wall-clock
// timeout, and the exact tool set the agent loop would see after the per-run
// filter — without executing anything or spending tokens. It is the payload of
// `agt run --dry-run`. Pure: deterministic in its input, no side effects.
func buildRunPlan(in runPlanInput) map[string]any {
	tenant := in.Tenant
	if strings.TrimSpace(tenant) == "" {
		tenant = "(primary)"
	}

	modelSource := "daemon default"
	if in.ModelOverridden {
		modelSource = "per-run (--model)"
	}

	systemSource := "none"
	if in.SystemSet {
		systemSource = "daemon default"
	}
	if in.SystemOverride {
		systemSource = "per-run (--system)"
	}

	timeout := "none"
	switch {
	case strings.TrimSpace(in.Timeout) != "":
		timeout = in.Timeout + " (per-run)"
	case in.DaemonTimeout > 0:
		timeout = in.DaemonTimeout.String() + " (daemon default)"
	}

	costCap := "none"
	if in.MaxCostMC > 0 {
		costCap = formatMicrocentsUSD(in.MaxCostMC) + " (per-run)"
	}

	// Effective tool set. Absent allow-list = the full kernel toolset. A present
	// allow-list intersects with the registered names (an unknown requested name
	// is reported under tools_dropped — it would surface as "tool X is not
	// available" at run time, but a dry-run flags it up front).
	registered := make(map[string]struct{}, len(in.AllToolNames))
	for _, n := range in.AllToolNames {
		registered[n] = struct{}{}
	}
	var effective, dropped []string
	toolsMode := "all"
	if in.AllowSet {
		toolsMode = "restricted"
		if len(in.Allow) == 0 {
			toolsMode = "none (--no-tools)"
		}
		seen := make(map[string]struct{}, len(in.Allow))
		for _, n := range in.Allow {
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			if _, ok := registered[n]; ok {
				effective = append(effective, n)
			} else {
				dropped = append(dropped, n)
			}
		}
	} else {
		effective = append(effective, in.AllToolNames...)
	}
	sort.Strings(effective)
	sort.Strings(dropped)
	if effective == nil {
		effective = []string{}
	}

	// Advisories (M160): preventive warnings a dry-run can surface before any
	// token is spent. The point is to catch a run that resolves cleanly but would
	// misbehave or be rejected at execution time.
	var warnings []string
	if !in.ModelKnown {
		warnings = append(warnings, fmt.Sprintf(
			"model %q is not in the catalog — its capabilities (vision, tool-use, "+
				"context window) are unverified; a run may fail in ways a dry-run can't predict",
			in.Model))
	} else {
		// Tool-use mismatch matters only when tools are actually enabled for this
		// run — with --no-tools the model never needs to call anything.
		if !in.SupportsTools && len(effective) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"model %q does not advertise tool-use (tool_call=false), but %d tool(s) are "+
					"enabled — calls may be ignored; under AGEZT_MODEL_STRICT=on this run would "+
					"be rejected before any provider call (see `agt provider check --caps`)",
				in.Model, len(effective)))
		}
		if in.ContextLimit > 0 && in.ContextLimit < smallContextThreshold {
			warnings = append(warnings, fmt.Sprintf(
				"model %q has a small context window (%d tokens) — long runs with memory/tools "+
					"may overflow it", in.Model, in.ContextLimit))
		}
	}
	// A cost cap only binds if the run accrues priced spend. On an unpriced model
	// (unknown to the catalog, or a free/local model with no cost) the cap can
	// never trip — surface that so the operator isn't lulled into thinking a run is
	// money-bounded when it isn't (M167).
	if in.MaxCostMC > 0 && !in.ModelPriced {
		warnings = append(warnings, fmt.Sprintf(
			"--max-cost %s is set, but model %q has no known pricing — the cap will not bind "+
				"(spend is computed as $0); `agt catalog sync` to load prices",
			formatMicrocentsUSD(in.MaxCostMC), in.Model))
	}

	plan := map[string]any{
		"dry_run":       true,
		"intent":        in.Intent,
		"tenant":        tenant,
		"model":         in.Model,
		"model_source":  modelSource,
		"model_known":   in.ModelKnown,
		"system_source": systemSource,
		"timeout":       timeout,
		"cost_cap":      costCap,
		"tools_mode":    toolsMode,
		"tools":         effective,
	}
	if in.ModelKnown {
		plan["supports_vision"] = in.SupportsVision
		plan["supports_tools"] = in.SupportsTools
	}
	if len(dropped) > 0 {
		plan["tools_dropped"] = dropped
	}
	if len(warnings) > 0 {
		plan["warnings"] = warnings
	}
	return plan
}
