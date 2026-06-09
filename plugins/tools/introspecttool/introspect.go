// SPDX-License-Identifier: MIT

// Package introspecttool is the in-process self-introspection tool: it lets the
// agent read the DAEMON's OWN live state in one call — a real health overview
// (uptime, halted, active runs, memory/world/skill counts, journal head,
// schedule/standing/approval posture, provider-fallback health, delegation
// ceilings), plus detailed listings of the schedules and standing orders that
// drive its autonomy (M682).
//
// This closes the introspection gap: the granular tools (memory, world, runs,
// skill) each read ONE slice, so a "summarise AGEZT's health every morning at 9"
// task had no single place to see the whole system and would resort to guessing
// (or web-searching "AGEZT"). `introspect` is that place — the agent can actually
// see everything that's running before it reports. Read-only; no mutation, no
// network.
package introspecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/standing"
)

// Delegation mirrors the active sub-agent governance ceilings (M46–M48) for the
// overview. 0 fan-out / spend / total = unbounded.
type Delegation struct {
	Enabled            bool  `json:"enabled"`
	MaxDepth           int   `json:"max_depth"`
	MaxFanout          int   `json:"max_fanout"`
	MaxSpendMicrocents int64 `json:"max_spend_microcents"`
	MaxTotal           int   `json:"max_total"`
}

// Overview is the daemon's at-a-glance health snapshot — the same shape `agt
// status` assembles, minus the server-level (HTTP/channel) extras the kernel
// doesn't own. Assembled by the Source (the kernel adapter); the tool just
// formats it.
type Overview struct {
	Daemon                 string     `json:"daemon"`
	Protocol               int        `json:"protocol"`
	Model                  string     `json:"model"`
	UptimeSeconds          int64      `json:"uptime_seconds"`
	Halted                 bool       `json:"halted"`
	ActiveRuns             int        `json:"active_runs"`
	Tools                  []string   `json:"tools"`
	MemoryRecords          int        `json:"memory_records"`
	WorldEntities          int        `json:"world_entities"`
	ActiveSkills           int        `json:"active_skills"`
	JournalHead            int64      `json:"journal_head"`
	SchedulesTotal         int        `json:"schedules_total"`
	SchedulesEnabled       int        `json:"schedules_enabled"`
	PendingApprovals       int        `json:"pending_approvals"`
	ProviderFallbacks      int        `json:"provider_fallbacks"`
	ProviderFallbackReason string     `json:"provider_fallback_reason,omitempty"`
	Delegation             Delegation `json:"delegation"`
}

// Source is the narrow live-state surface the tool reads. The kernel adapter
// (NewKernelSource) implements it against the real *runtime.Kernel; tests inject
// a fake without standing up a daemon.
type Source interface {
	// Overview returns the daemon's at-a-glance health snapshot.
	Overview() Overview
	// Schedules returns every cadence entry (operator- and agent-created).
	Schedules() []cadence.Entry
	// Standing returns every standing order on this daemon.
	Standing() []standing.Order
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the Source.
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

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "introspect",
		Description: "Read THIS daemon's OWN live state — use this to report on AGEZT's health " +
			"instead of guessing. op=overview (default) gives the at-a-glance snapshot: version, " +
			"model, uptime, halted, active runs, registered tools, memory/world/skill counts, " +
			"journal head, schedule & standing-order & pending-approval counts, provider-fallback " +
			"health, and delegation ceilings. op=schedules lists the scheduled (time-driven) runs " +
			"in detail; op=standing lists the standing orders (event/cron-triggered autonomous " +
			"agents) in detail. For deeper drill-down combine with the runs, memory, world and " +
			"skill tools.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "op": {"type":"string", "enum":["overview","schedules","standing"], "description":"What to read (default overview)."}
  }
}`),
	}
}

type input struct {
	Op string `json:"op"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return agent.Result{}, fmt.Errorf("introspect: parse input: %w", err)
		}
	}
	if t.src == nil {
		return errResult("introspection is not available on this daemon"), nil
	}

	switch in.Op {
	case "", "overview":
		return okJSON(t.src.Overview()), nil
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
		return errResult("unknown op " + in.Op + " (overview|schedules|standing)"), nil
	}
}

// scheduleView renders one cadence entry for the agent — the human-readable
// cadence string plus the load-bearing fields, omitting zero/empty noise.
func scheduleView(e cadence.Entry) map[string]any {
	v := map[string]any{
		"id":            e.ID,
		"intent":        e.Intent,
		"cadence":       e.Cadence(),
		"mode":          e.Mode,
		"source":        e.Source,
		"enabled":       e.Enabled,
		"next_run_unix": e.NextRunUnix,
	}
	if e.LastRunUnix > 0 {
		v["last_run_unix"] = e.LastRunUnix
	}
	if e.Fires > 0 {
		v["fires"] = e.Fires
	}
	if e.Model != "" {
		v["model"] = e.Model
	}
	if e.Assure > 0 {
		v["assure"] = e.Assure
	}
	return v
}

func next(v map[string]any) int64 {
	if n, ok := v["next_run_unix"].(int64); ok {
		return n
	}
	return 0
}

// standingView renders one standing order — mirrors the standing tool's view so
// the two read identically.
func standingView(o standing.Order) map[string]any {
	trigs := make([]map[string]any, 0, len(o.Triggers))
	for _, tr := range o.Triggers {
		m := map[string]any{"type": string(tr.Type)}
		if tr.Schedule != "" {
			m["schedule"] = tr.Schedule
		}
		if tr.Subject != "" {
			m["subject"] = tr.Subject
		}
		trigs = append(trigs, m)
	}
	v := map[string]any{
		"id": o.ID, "name": o.Name, "enabled": o.Enabled,
		"mode": string(o.Initiative.Mode), "triggers": trigs, "plan": o.Plan,
	}
	if o.Assure > 0 {
		v["assure"] = o.Assure
	}
	return v
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "introspect: " + msg, IsError: true}
}
