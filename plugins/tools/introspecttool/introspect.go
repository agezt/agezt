// SPDX-License-Identifier: MIT

package introspecttool

// This file owns the public Tool surface (types + Bind / Definition /
// Invoke). The render helpers (scheduleView, standingView, okJSON,
// errResult) live in introspect_views.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
)

// The rows live in the contract layer (kernel/contract/introspectapi): the
// tool is a PLUGIN, and a plugin may reach the contract and the platform
// layers and nothing else — so a row a plugin publishes cannot live in the
// plugin itself, and the implementation that assembles it (the app layer's
// kernel adapter) must be able to hand it over without importing the plugin.
//
// The names stay here as aliases, so every existing caller is unchanged.
type (
	// Delegation mirrors the active sub-agent governance ceilings (M46–M48) for the
	// overview. 0 fan-out / spend / total = unbounded. It is the contract type
	// itself, so the overview carries the runtime's ceilings unchanged.
	Delegation = introspectapi.Delegation
	// ReaperOverview is the daemon's stale/degraded/misconfigured fleet snapshot.
	ReaperOverview = introspectapi.ReaperOverview
	// ReaperAgent is one dead agent in the reaper's scan.
	ReaperAgent = introspectapi.ReaperAgent
	// ReaperDegradedAgent is one degraded agent in the reaper's scan.
	ReaperDegradedAgent = introspectapi.ReaperDegradedAgent
	// ReaperMisconfiguredAgent is one misconfigured agent in the reaper's scan.
	ReaperMisconfiguredAgent = introspectapi.ReaperMisconfiguredAgent
	// ReaperReport is the reaper's full fleet scan.
	ReaperReport = introspectapi.ReaperReport
	// Overview is the daemon's at-a-glance health snapshot — the same shape `agt
	// status` assembles, minus the server-level (HTTP/channel) extras the kernel
	// doesn't own. Assembled by the Source (the kernel adapter); the tool just
	// formats it.
	Overview = introspectapi.Overview
)

// Source is the narrow live-state surface the tool reads. It is the
// contract's PORT: the tool is a plugin, so it may not name the kernel, and
// the app layer's adapter (kernel/app/introspect) implements the port against
// the live kernel. Tests inject a fake without standing up a daemon.
type Source = introspectapi.Source

// Tool implements toolapi.Tool. Created unbound via New(); Bind wires the Source.
type Tool struct {
	src Source
}

// New returns an unbound introspect tool (no Source until Bind).
func New() *Tool { return &Tool{} }

// Bind wires the live state source. Called once after the kernel opens.
func (t *Tool) Bind(s Source) {
	if s != nil {
		t.src = s
	}
}

// Definition implements toolapi.Tool.
func (t *Tool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:       "introspect",
		Capability: toolapi.ToolCapability{Name: string(edict.CapIntrospect)},
		Description: "Read THIS daemon's OWN live state — use this to report on AGEZT's health " +
			"instead of guessing. op=overview (default) gives the at-a-glance snapshot: version, " +
			"model, uptime, halted, active runs, registered tools, memory/world/skill counts, " +
			"journal head, schedule & standing-order & pending-approval counts, provider-fallback " +
			"health, reaper candidates (dead/degraded/misconfigured agents), and delegation ceilings. op=reaper gives the full current reaper scan. op=schedules lists the scheduled (time-driven) runs " +
			"in detail; op=standing lists the standing orders (event/cron-triggered autonomous " +
			"agents) in detail. For deeper drill-down combine with the runs, memory, world and " +
			"skill tools.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "op": {"type":"string", "enum":["overview","reaper","schedules","standing"], "description":"What to read (default overview)."}
  }
}`),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectReversible,
			PredictedEffects: []string{
				"Read the daemon's current health, schedule, standing-order, approval, and delegation posture.",
			},
			AffectedResources: []string{"daemon runtime state snapshot"},
			RollbackNotes:     "Read-only; no rollback required.",
			Confidence:        0.95,
		},
	}
}

type input struct {
	Op string `json:"op"`
}

// Invoke implements toolapi.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (toolapi.Result, error) {
	var in input
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolapi.Result{}, fmt.Errorf("introspect: parse input: %w", err)
		}
	}
	if t.src == nil {
		return errResult("introspection is not available on this daemon"), nil
	}

	switch in.Op {
	case "", "overview":
		return okJSON(t.src.Overview()), nil
	case "reaper":
		return okJSON(t.src.Reaper()), nil
	case "schedules":
		entries := t.src.Schedules()
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			out = append(out, scheduleView(e))
		}
		// Soonest next run first so the most imminent autonomy is at the top.
		sort.SliceStable(out, func(i, j int) bool {
			return next(out[i]) < next(out[j])
		})
		return okJSON(map[string]any{"count": len(out), "schedules": out}), nil
	case "standing":
		orders := t.src.Standing()
		out := make([]map[string]any, 0, len(orders))
		for _, o := range orders {
			out = append(out, standingView(o))
		}
		return okJSON(map[string]any{"count": len(out), "orders": out}), nil
	default:
		return errResult("unknown op " + in.Op + " (overview|reaper|schedules|standing)"), nil
	}
}

// scheduleView renders one cadence entry for the agent — the human-readable
// cadence string plus the load-bearing fields, omitting zero/empty noise.
