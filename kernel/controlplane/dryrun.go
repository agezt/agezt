// SPDX-License-Identifier: MIT

package controlplane

import (
	"sort"
	"strings"
	"time"
)

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
	SystemSet       bool   // a daemon-default system prompt is configured
	SystemOverride  bool   // a per-run --system was given
	Timeout         string // per-run timeout (raw, validated); "" if none
	DaemonTimeout   time.Duration
	AllToolNames    []string // the kernel's full toolset (names)
	AllowSet        bool     // a "tools" arg was present (restriction in effect)
	Allow           []string // requested tool names (the allow-list)
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

	plan := map[string]any{
		"dry_run":       true,
		"intent":        in.Intent,
		"tenant":        tenant,
		"model":         in.Model,
		"model_source":  modelSource,
		"model_known":   in.ModelKnown,
		"system_source": systemSource,
		"timeout":       timeout,
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
	return plan
}
