// SPDX-License-Identifier: MIT
//
// kernel/controlplane dry-run plan builder (buildRunPlan) + consts
// (smallContextThreshold, microcentsPerUSD) + runPlanInput type.
package controlplane

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

// smallContextThreshold mirrors catalog.Model.AgentWarnings: a context window
// below this is flagged as a risk for long agent runs with memory + tools.
const smallContextThreshold = 8192

// microcentsPerUSD is the kernel's spend unit: $1 = 1e9 USD-microcents (matches
// governor.DefaultDailyCeilingMicrocents and the CLI's --max-cost parser).
const microcentsPerUSD = 1_000_000_000

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
	executionProfile := strings.TrimSpace(in.ExecutionProfile)
	executionSource := "tool defaults"
	wardenProfile := "tool defaults"
	if executionProfile != "" {
		executionSource = "per-run (--exec-profile)"
		if strings.TrimSpace(in.WardenProfile) != "" {
			wardenProfile = in.WardenProfile
		}
	} else {
		executionProfile = "(tool defaults)"
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
	// Strict pricing (M195): when the daemon refuses unpriced models
	// (AGEZT_PRICING_STRICT=on), a run on a model with no KNOWN price is rejected
	// before any provider call. Surface that here so the operator learns it from a
	// dry-run instead of a surprising "model has no known price" failure at submit.
	// Uses ModelHasPrice (known incl. free), NOT ModelPriced (cost > 0) — a
	// known-free model is priced and would NOT be refused.
	if in.StrictPricing && in.Model != "" && !in.ModelHasPrice {
		warnings = append(warnings, fmt.Sprintf(
			"strict pricing is on (%sPRICING_STRICT) and model %q has no known price — this run "+
				"would be REFUSED before any provider call; `agt catalog sync` to load prices, or "+
				"unset the flag", brand.EnvPrefix, in.Model))
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
		"dry_run":                  true,
		"intent":                   in.Intent,
		"tenant":                   tenant,
		"model":                    in.Model,
		"model_source":             modelSource,
		"model_known":              in.ModelKnown,
		"system_source":            systemSource,
		"timeout":                  timeout,
		"cost_cap":                 costCap,
		"execution_profile":        executionProfile,
		"execution_profile_source": executionSource,
		"warden_profile":           wardenProfile,
		"tools_mode":               toolsMode,
		"tools":                    effective,
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
	if peer := strings.TrimSpace(in.RemotePeer); peer != "" {
		plan["remote_peer"] = peer
	}
	return plan
}
