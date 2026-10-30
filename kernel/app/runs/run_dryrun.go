// SPDX-License-Identifier: MIT

package runs

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/runtime"
)

// smallContextThreshold mirrors catalog.Model.AgentWarnings: a context window
// below this is flagged as a risk for long agent runs with memory + tools.
const smallContextThreshold = 8192

// microcentsPerUSD is the kernel's spend unit: $1 = 1e9 USD-microcents (matches
// governor.DefaultDailyCeilingMicrocents and the CLI's --max-cost parser).
const microcentsPerUSD = 1_000_000_000

// dryRunInput carries the already-resolved primitives for a run, so the plan
// builder stays pure (no kernel or catalog handles) and table-testable.
type dryRunInput struct {
	Intent           string
	Tenant           string // "" = primary kernel
	Model            string // the effective model this run would use
	ModelOverridden  bool   // true if a per-run model was given
	ModelKnown       bool   // the catalog knows the effective model
	SupportsVision   bool   // the catalog's vision cap (meaningful only when ModelKnown)
	SupportsTools    bool   // the catalog's tool_call cap (meaningful only when ModelKnown)
	ContextLimit     int    // the model's context window in tokens (0 = unknown)
	SystemSet        bool   // a daemon-default system prompt is configured
	SystemOverride   bool   // a per-run system prompt was given
	Timeout          string // the per-run timeout (raw, validated); "" if none
	DaemonTimeout    time.Duration
	AllToolNames     []string // the kernel's effective toolset (names)
	AllowSet         bool     // a "tools" arg was present (restriction in effect)
	Allow            []string // the requested tool names (the allow-list)
	MaxCostMC        int64    // the per-run cost cap in microcents (0 = none)
	ExecutionProfile string   // the per-run execution profile id; "" = tool defaults
	WardenProfile    string   // the requested warden profile when ExecutionProfile is set
	RemotePeer       string   // the requested remote-agezt peer, if pinned
	ModelPriced      bool     // the model prices to > $0 (a cap could trip)
	StrictPricing    bool     // the daemon refuses unpriced models (AGEZT_PRICING_STRICT)
	ModelHasPrice    bool     // the model has a KNOWN price entry (incl. known-free)
}

// dryRun reports what the resolved run WOULD do: the effective model and its
// catalog capabilities, the source of the system prompt, the effective
// wall-clock timeout, and the precise tool set the agent loop would see after
// the per-run filter. Nothing runs and no token is spent.
func (s *RunService) dryRun(resolved *resolvedRun) (RunOutput, error) {
	in := dryRunInput{
		Intent:           resolved.intent,
		Tenant:           resolved.tenant,
		Model:            resolved.model,
		ModelOverridden:  resolved.overridden,
		SystemSet:        strings.TrimSpace(s.ports.Prompt()) != "",
		SystemOverride:   resolved.system != "",
		Timeout:          resolved.timeout,
		DaemonTimeout:    s.ports.Ceiling(),
		AllowSet:         resolved.toolsSet,
		Allow:            resolved.tools,
		MaxCostMC:        resolved.maxCost,
		ExecutionProfile: resolved.profile,
		WardenProfile:    resolved.warden,
		RemotePeer:       resolved.peer,
		ModelPriced:      modelPriced(resolved.model),
	}
	in.StrictPricing, in.ModelHasPrice = strictPricingPlan(s.ports.LiveProvider(), resolved.model)
	if cat := s.ports.CatalogSnapshot(); cat != nil {
		if _, m := cat.FindModel(resolved.model); m != nil {
			in.ModelKnown = true
			in.SupportsVision = m.SupportsVision()
			in.SupportsTools = m.ToolCall
			in.ContextLimit = m.Limit.Context
		}
	}
	for name := range runtime.AgentTools(resolved.ctx, s.ports.RegisteredTools()) {
		in.AllToolNames = append(in.AllToolNames, name)
	}
	return buildRunPlan(in), nil
}

// buildRunPlan resolves what a run WOULD do — effective model (and its catalog
// capabilities), the source of the system prompt, the effective wall-clock
// timeout, and the exact tool set the agent loop would see after the per-run
// filter — without executing anything or spending tokens. It is the payload of
// `agt run --dry-run`. Pure: deterministic in its input, no side effects.
func buildRunPlan(in dryRunInput) RunOutput {
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

	// The effective tool set. An absent allow-list is the full kernel toolset.
	// A present allow-list intersects with the registered names (an unknown
	// requested name is reported under tools_dropped — it would surface as
	// "tool X is not available" at run time, but a dry-run flags it up front).
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

	// Preventive warnings a dry-run can surface before any token is spent: a
	// run that resolves cleanly but would misbehave or be rejected at
	// execution time.
	var warnings []string
	if !in.ModelKnown {
		warnings = append(warnings, fmt.Sprintf(
			"model %q is not in the catalog — its capabilities (vision, tool-use, "+
				"context window) are unverified; a run may fail in ways a dry-run can't predict",
			in.Model))
	} else {
		// A tool-use mismatch matters only when tools are actually enabled for
		// this run — with no tools the model never needs to call anything.
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
	// Strict pricing: when the daemon refuses unpriced models, a run on a
	// model with no KNOWN price is rejected before any provider call. This
	// uses ModelHasPrice (known incl. free), NOT ModelPriced (cost > 0) — a
	// known-free model is priced and would NOT be refused.
	if in.StrictPricing && in.Model != "" && !in.ModelHasPrice {
		warnings = append(warnings, fmt.Sprintf(
			"strict pricing is on (%sPRICING_STRICT) and model %q has no known price — this run "+
				"would be REFUSED before any provider call; `agt catalog sync` to load prices, or "+
				"unset the flag", brand.EnvPrefix, in.Model))
	}
	// A cost cap only binds if the run accrues priced spend. On an unpriced
	// model the cap can never trip — surface that so the operator isn't
	// lulled into thinking a run is money-bounded when it isn't.
	if in.MaxCostMC > 0 && !in.ModelPriced {
		warnings = append(warnings, fmt.Sprintf(
			"--max-cost %s is set, but model %q has no known pricing — the cap will not bind "+
				"(spend is computed as $0); `agt catalog sync` to load prices",
			formatMicrocentsUSD(in.MaxCostMC), in.Model))
	}

	out := RunOutput{
		DryRun:           boolPtr(true),
		Intent:           stringPtr(in.Intent),
		Tenant:           stringPtr(tenant),
		Model:            stringPtr(in.Model),
		ModelSource:      stringPtr(modelSource),
		ModelKnown:       boolPtr(in.ModelKnown),
		SystemSource:     stringPtr(systemSource),
		Timeout:          stringPtr(timeout),
		CostCap:          stringPtr(costCap),
		ExecutionProfile: stringPtr(executionProfile),
		ExecutionSource:  stringPtr(executionSource),
		WardenProfile:    stringPtr(wardenProfile),
		ToolsMode:        stringPtr(toolsMode),
		Tools:            &effective,
	}
	if in.ModelKnown {
		out.SupportsVision = boolPtr(in.SupportsVision)
		out.SupportsTools = boolPtr(in.SupportsTools)
	}
	if len(dropped) > 0 {
		out.ToolsDropped = &dropped
	}
	if len(warnings) > 0 {
		out.Warnings = &warnings
	}
	if peer := strings.TrimSpace(in.RemotePeer); peer != "" {
		out.RemotePeer = stringPtr(peer)
	}
	return out
}

// formatMicrocentsUSD renders a microcent amount as US dollars.
func formatMicrocentsUSD(mc int64) string {
	return fmt.Sprintf("$%.2f", float64(mc)/microcentsPerUSD)
}

func stringPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }
