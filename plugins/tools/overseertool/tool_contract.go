// SPDX-License-Identifier: MIT

// Overseer tool: Definition (the agent.Tool contract) + input struct (the
// payload shape). The Invoke dispatcher lives in tool.go; helpers (parseProfile,
// agentView, okJSON, ...) live in tool_helpers.go. Day-211 god-file split.
// Public API unchanged.
package overseertool


import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)
// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:       "overseer",
		Capability: agent.ToolCapability{Name: string(edict.CapOversee)},
		Description: "Supervise and intervene on the whole system — the brain/overseer's controls. " +
			"op=status shows the daemon's health (halted?, active runs, agent count, open help); " +
			"op=agents lists every agent with its state (enabled/paused/retired) and model; " +
			"op=runs lists the runs in flight right now (correlation ids you can cancel); " +
			"op=help lists the open help requests waiting for an answer (triage). " +
			"Intervene: op=cancel stops one run by its correlation id; op=halt stops ALL runs and blocks " +
			"new ones until op=resume; op=pause/op=unpause pause or resume a named agent; op=retire moves " +
			"an agent to the graveyard (op=impact first to see what depends on it) and op=revive brings it " +
			"back; op=delete permanently removes an agent from the roster. Inspect: op=get shows the full " +
			"profile of one agent; op=search finds agents by state, model, task type, or owner. " +
			"Treat the fleet: op=edit retunes another agent (identity, budgets, policy, " +
			"config_overrides via the \"profile\" object), op=create makes a new agent, op=clone duplicates an " +
			"existing agent with overrides (template-based creation), op=wake triggers an agent " +
			"asynchronously, and op=repair runs a governed self-repair " +
			"pass AS a named agent and auto-applies a closing profile proposal when present. Every action is " +
			"journaled and reversible. " +
			"Use this to keep the fleet healthy: stop a runaway, pause or retune a misbehaving agent, fix a " +
			"hot model, delete defunct identities permanently, answer or route a help request.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":     {"type":"string", "enum":["status","agents","runs","help","cancel","halt","resume","pause","unpause","retire","revive","impact","edit","create","delete","get","clone","search","bulk_pause","bulk_unpause","bulk_retire","bulk_revive","bulk_delete","wake","repair"]},
    "agent":  {"type":"string", "description":"For op=pause/unpause/retire/revive/impact/edit/repair/delete/get/wake: the target agent's slug (or id)."},
    "run":    {"type":"string", "description":"For op=cancel: the correlation id of the run to stop (from op=runs)."},
    "source": {"type":"string", "description":"For op=clone: the source agent's slug (or id) to copy fields from."},
    "reason": {"type":"string", "description":"For op=halt/resume/cancel/retire/delete/repair/wake/bulk_retire (optional): why — recorded in the journal, graveyard entry, wake intent, or included in the repair brief."},
    "limit":  {"type":"integer", "description":"For op=help: max requests to list (default 20)."},
    "profile":{"type":"object", "description":"For op=edit/create/clone: agent fields to apply. Keys include slug (create/clone required), name, soul, model, fallbacks (array), task_type, max_cost_mc, max_daily_mc, memory_scope, workdir, description, tool_allow, tool_deny, trust_ceiling, retry_policy, health_policy, self_repair, noise_policy, config_overrides. op=edit applies them wholesale to the target named by \"agent\"; op=clone applies overrides on top of the source profile."},
    "filter":{"type":"object", "description":"For op=search: filter criteria object. Keys include query (substring match on slug/name/description), state (enabled|paused|retired), model, task_type, system (bool), has_owner (bool), has_parent (bool), tool_allowed, limit (max results, default 100). All keys are optional; empty filter returns all non-retired agents."},
    "intent": {"type":"string", "description":"For op=wake: the run intent (optional — falls back to reason)."},
    "agents": {"type":"array", "items":{"type":"string"}, "description":"For op=bulk_pause/unpause/retire/replicate/delete: list of agent slugs to operate on."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Read fleet health, active runs, help requests, and agent profiles.",
				"Cancel or halt runs and create, edit, pause, retire, or revive agents for mutating operations.",
			},
			AffectedResources: []string{"active run controls", "global halt switch", "agent roster", "open help queue"},
			RollbackNotes:     "Resume halted runs acceptance, unpause/revive agents, or edit profiles back; cancelled in-flight work cannot be resumed and must be rerun.",
			Confidence:        0.75,
		},
	}
}

type input struct {
	Op      string          `json:"op"`
	Agent   string          `json:"agent"`
	Run     string          `json:"run"`
	Source  string          `json:"source"`
	Reason  string          `json:"reason"`
	Intent  string          `json:"intent"`
	Limit   int             `json:"limit"`
	Profile json.RawMessage `json:"profile"`
	Filter  *SearchFilter   `json:"filter,omitempty"`
	Agents  []string        `json:"agents,omitempty"`
}
