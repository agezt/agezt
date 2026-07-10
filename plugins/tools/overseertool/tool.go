// SPDX-License-Identifier: MIT

package overseertool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

const defaultHelpLimit = 20

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "overseer",
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

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("overseer: parse input: %w", err)
	}
	s := t.current()
	if s == nil {
		return errResult("the overseer is not available on this daemon"), nil
	}
	op := strings.ToLower(strings.TrimSpace(in.Op))

	switch op {
	case "status":
		return okJSON(map[string]any{
			"halted":      s.IsHalted(),
			"active_runs": len(s.ActiveRunIDs()),
			"agents":      len(s.Agents()),
			"open_help":   len(s.OpenHelp(0)),
		}), nil

	case "agents":
		ags := s.Agents()
		views := make([]map[string]any, 0, len(ags))
		for _, p := range ags {
			views = append(views, agentView(p))
		}
		return okJSON(map[string]any{"count": len(views), "agents": views}), nil

	case "runs":
		ids := s.ActiveRunIDs()
		return okJSON(map[string]any{"count": len(ids), "active_runs": ids,
			"hint": "stop one with op=cancel run=<id>"}), nil

	case "help":
		limit := in.Limit
		if limit <= 0 {
			limit = defaultHelpLimit
		}
		open := s.OpenHelp(limit)
		views := make([]map[string]any, 0, len(open))
		for _, m := range open {
			views = append(views, helpView(m))
		}
		return okJSON(map[string]any{"count": len(views), "open_help": views,
			"hint": "answer one with the board tool: op=reply id=<id>"}), nil

	case "cancel":
		if strings.TrimSpace(in.Run) == "" {
			return errResult(`op=cancel needs "run" (the correlation id from op=runs)`), nil
		}
		ok := s.CancelRun(strings.TrimSpace(in.Run))
		return okJSON(map[string]any{"run": in.Run, "cancelled": ok,
			"note": cancelNote(ok)}), nil

	case "halt":
		s.Halt(strings.TrimSpace(in.Reason))
		return okJSON(map[string]any{"halted": true,
			"note": "all in-flight runs cancelled; new runs are refused until op=resume"}), nil

	case "resume":
		s.ResumeAll(strings.TrimSpace(in.Reason))
		return okJSON(map[string]any{"halted": false, "note": "the daemon accepts runs again"}), nil

	case "pause", "unpause":
		if strings.TrimSpace(in.Agent) == "" {
			return errResult("op=" + op + ` needs "agent" (the target slug)`), nil
		}
		enabled := op == "unpause"
		p, err := s.SetAgentEnabled(strings.TrimSpace(in.Agent), enabled)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "enabled": p.Enabled,
			"action": map[bool]string{true: "resumed", false: "paused"}[enabled]}), nil

	case "retire":
		if strings.TrimSpace(in.Agent) == "" {
			return errResult(`op=retire needs "agent" (the target slug)`), nil
		}
		impact := s.AgentImpact(strings.TrimSpace(in.Agent))
		p, err := s.SetAgentRetired(strings.TrimSpace(in.Agent), true, strings.TrimSpace(in.Reason))
		if err != nil {
			return errResult(err.Error()), nil
		}
		out := map[string]any{"agent": p.Slug, "retired": true, "action": "retired"}
		if len(impact) > 0 {
			out["impact"] = impact
		}
		return okJSON(out), nil

	case "revive":
		if strings.TrimSpace(in.Agent) == "" {
			return errResult(`op=revive needs "agent" (the target slug)`), nil
		}
		p, err := s.SetAgentRetired(strings.TrimSpace(in.Agent), false, "")
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "retired": false, "action": "revived"}), nil

	case "delete":
		ref := strings.TrimSpace(in.Agent)
		if ref == "" {
			return errResult(`op=delete needs "agent" (the target slug or id)`), nil
		}
		ok, err := s.DeleteAgent(ref)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": ref, "action": "deleted", "removed": ok}), nil

	case "get":
		ref := strings.TrimSpace(in.Agent)
		if ref == "" {
			return errResult(`op=get needs "agent" (the target slug or id)`), nil
		}
		p, ok, err := s.GetAgent(ref)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !ok {
			return errResult("unknown agent: " + ref), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "profile": agentView(p)}), nil

	case "clone":
		source := strings.TrimSpace(in.Source)
		if source == "" {
			return errResult(`op=clone needs "source" (the existing agent slug)`), nil
		}
		prof, perr := parseProfile(in.Profile, raw)
		if perr != nil {
			return errResult(perr.Error()), nil
		}
		if strings.TrimSpace(prof.Slug) == "" {
			return errResult(`op=clone needs a "profile" object with a "slug" for the new agent`), nil
		}
		p, err := s.CloneAgent(source, prof)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "action": "cloned", "source": source, "profile": agentView(p)}), nil

	case "search":
		filter := in.Filter
		if filter == nil {
			filter = &SearchFilter{}
		}
		results := s.SearchAgents(*filter)
		views := make([]map[string]any, 0, len(results))
		for _, p := range results {
			views = append(views, agentView(p))
		}
		return okJSON(map[string]any{"count": len(views), "agents": views}), nil

	case "bulk_pause", "bulk_unpause":
		slugs := cleanSlugs(in.Agents)
		if len(slugs) == 0 {
			return errResult("op=" + op + ` needs "agents" (array of slugs)`), nil
		}
		enabled := op == "bulk_unpause"
		results := s.BulkSetEnabled(slugs, enabled)
		return okJSON(map[string]any{"op": op, "total": len(results), "results": results}), nil

	case "bulk_retire", "bulk_revive":
		slugs := cleanSlugs(in.Agents)
		if len(slugs) == 0 {
			return errResult("op=" + op + ` needs "agents" (array of slugs)`), nil
		}
		retired := op == "bulk_retire"
		results := s.BulkSetRetired(slugs, retired, strings.TrimSpace(in.Reason))
		return okJSON(map[string]any{"op": op, "total": len(results), "results": results}), nil

	case "bulk_delete":
		slugs := cleanSlugs(in.Agents)
		if len(slugs) == 0 {
			return errResult(`op=bulk_delete needs "agents" (array of slugs)`), nil
		}
		results := s.BulkDelete(slugs)
		return okJSON(map[string]any{"op": op, "total": len(results), "results": results}), nil

	case "wake":
		ref := strings.TrimSpace(in.Agent)
		if ref == "" {
			return errResult(`op=wake needs "agent" (the target slug)`), nil
		}
		intent := strings.TrimSpace(in.Intent)
		reason := strings.TrimSpace(in.Reason)
		if intent == "" && reason == "" {
			return errResult(`op=wake needs "intent" or "reason"`), nil
		}
		if intent == "" {
			intent = reason
		}
		corr, err := s.WakeAgent(ref, intent, reason)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": ref, "correlation_id": corr, "action": "woken"}), nil

	case "impact":
		if strings.TrimSpace(in.Agent) == "" {
			return errResult(`op=impact needs "agent" (the target slug)`), nil
		}
		impact := s.AgentImpact(strings.TrimSpace(in.Agent))
		return okJSON(map[string]any{"agent": in.Agent, "standing_orders": impact, "count": len(impact)}), nil

	case "edit":
		ref := strings.TrimSpace(in.Agent)
		if ref == "" {
			return errResult(`op=edit needs "agent" (the target slug) and a "profile" object`), nil
		}
		prof, perr := parseProfile(in.Profile, raw)
		if perr != nil {
			return errResult(perr.Error()), nil
		}
		p, err := s.EditAgent(ref, prof)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "action": "edited", "profile": agentView(p)}), nil

	case "create":
		prof, perr := parseProfile(in.Profile, raw)
		if perr != nil {
			return errResult(perr.Error()), nil
		}
		if strings.TrimSpace(prof.Slug) == "" {
			return errResult(`op=create needs a "profile" object with a "slug"`), nil
		}
		p, err := s.CreateAgent(prof)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "action": "created", "profile": agentView(p)}), nil

	case "repair":
		ref := strings.TrimSpace(in.Agent)
		if ref == "" {
			return errResult(`op=repair needs "agent" (the target slug)`), nil
		}
		res, err := s.RepairAgent(ref, strings.TrimSpace(in.Reason))
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"agent":                             res.Agent,
			"action":                            "repair",
			"correlation":                       res.Correlation,
			"applied":                           res.Applied,
			"routing_task_type":                 res.RoutingTaskType,
			"routing_task_model_chain":          res.RoutingTaskModelChain,
			"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
			"answer":                            res.Answer,
		}), nil

	case "":
		return errResult("op required (status|agents|runs|help|cancel|halt|resume|pause|unpause|retire|revive|impact|edit|create|delete|get|clone|search|repair)"), nil
	default:
		return errResult("unknown op " + op), nil
	}
}

// overseerControlKeys are the top-level input keys that are NOT profile fields,
// so they're ignored when a model flattens the profile onto the tool input.
var overseerControlKeys = map[string]bool{
	"op": true, "agent": true, "run": true, "source": true, "reason": true, "limit": true, "profile": true, "filter": true, "agents": true,
}

// parseProfile decodes the op=edit/create profile into a roster.Profile. The
// canonical shape is a nested "profile" object, but some models flatten the
// fields straight onto the tool input (e.g. {"op":"create","slug":"x",...})
// instead of nesting them — so when "profile" is missing/empty we fall back to
// reading the profile fields off the top-level input. Either shape works; only
// a genuinely empty payload (no profile object AND no flat fields) is an error,
// so a guardian still can't silently no-op an edit.
func parseProfile(profileRaw, fullRaw json.RawMessage) (roster.Profile, error) {
	if hasProfileObject(profileRaw) {
		var p roster.Profile
		if err := json.Unmarshal(profileRaw, &p); err != nil {
			return roster.Profile{}, fmt.Errorf("profile: %w", err)
		}
		return p, nil
	}
	if hasFlatProfileFields(fullRaw) {
		var p roster.Profile
		if err := json.Unmarshal(fullRaw, &p); err != nil {
			return roster.Profile{}, fmt.Errorf("profile: %w", err)
		}
		return p, nil
	}
	return roster.Profile{}, fmt.Errorf(`a "profile" object is required (nest the agent fields under "profile", or pass them as top-level keys like "slug"/"name"/"soul"/"model")`)
}

// hasProfileObject reports whether raw is a populated "profile" object (not
// absent, null, or empty).
func hasProfileObject(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}"
}

// hasFlatProfileFields reports whether the top-level input carries any key that
// isn't a control key — i.e. the model flattened profile fields onto the input.
func hasFlatProfileFields(fullRaw json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(fullRaw, &m) != nil {
		return false
	}
	for k := range m {
		if !overseerControlKeys[k] {
			return true
		}
	}
	return false
}

func cancelNote(ok bool) string {
	if ok {
		return "the run was in flight and has been cancelled"
	}
	return "no in-flight run matched that id (already finished or unknown)"
}

func agentView(p roster.Profile) map[string]any {
	state := "enabled"
	switch {
	case p.Retired:
		state = "retired"
	case !p.Enabled:
		state = "paused"
	}
	v := map[string]any{"slug": p.Slug, "state": state, "enabled": p.Enabled, "retired": p.Retired}
	if p.Name != "" {
		v["name"] = p.Name
	}
	if p.Model != "" {
		v["model"] = p.Model
	}
	return v
}

func helpView(m board.Message) map[string]any {
	v := map[string]any{"id": m.ID, "text": m.Text}
	if m.From != "" {
		v["from"] = m.From
	}
	if m.To != "" {
		v["to"] = m.To
	}
	if m.TSMS > 0 {
		v["at"] = time.UnixMilli(m.TSMS).Format(time.RFC3339)
	}
	return v
}

// cleanSlugs trims whitespace from each slug and removes empties.
func cleanSlugs(slugs []string) []string {
	if len(slugs) == 0 {
		return nil
	}
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "overseer: " + msg, IsError: true}
}
