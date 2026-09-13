// SPDX-License-Identifier: MIT

// Overseer tool: Invoke (the op dispatcher) + overseerControlKeys (the input
// key allowlist) + defaultHelpLimit const. Definition + input struct moved to
// tool_contract.go. Helpers (parseProfile, agentView, okJSON, ...) live in
// tool_helpers.go. Day-211 god-file split. Public API unchanged.
package overseertool


import (
	"context"
	"fmt"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
)
const defaultHelpLimit = 20
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
		// System guardians are off-limits to the agent-reachable path (PE-006),
		// matching op=edit. A repair RUNS the target agent against a brief, and
		// the resolution it produces can rewrite the target's own Soul — which
		// boot reconcile does not re-clamp. So without this, an arbitrary agent
		// could aim a repair at a guardian and behaviourally defang the very
		// fleet that supervises it, going around the op=edit guard rather than
		// through it. Auto-repair already excludes System agents for the same
		// reason (kernel/selfrepair's claim filter).
		//
		// Guarded HERE and not in kernelSource.RepairAgent: the operator's
		// console/CLI repair button goes through that same method, and an
		// operator repairing their own guardian is legitimate. Only the
		// agent-reachable tool path is restricted — the same split op=edit
		// documents.
		if target, found, err := s.GetAgent(ref); err != nil {
			return errResult(err.Error()), nil
		} else if found && target.System {
			return errResult("agent " + target.Slug + " is a protected system guardian — it can be repaired only by an operator, not via the overseer tool"), nil
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
