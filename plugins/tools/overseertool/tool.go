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
			"back. Treat the fleet: op=edit retunes another agent (soul/model/fallbacks/budgets via the " +
			"\"profile\" object) and op=create makes a new agent. Every action is journaled and reversible. " +
			"Use this to keep the fleet healthy: stop a runaway, pause or retune a misbehaving agent, fix a " +
			"hot model, answer or route a help request.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":     {"type":"string", "enum":["status","agents","runs","help","cancel","halt","resume","pause","unpause","retire","revive","impact","edit","create"]},
    "agent":  {"type":"string", "description":"For op=pause/unpause/retire/revive/impact/edit: the target agent's slug (or id)."},
    "run":    {"type":"string", "description":"For op=cancel: the correlation id of the run to stop (from op=runs)."},
    "reason": {"type":"string", "description":"For op=halt/resume/cancel (optional): why — recorded in the journal."},
    "limit":  {"type":"integer", "description":"For op=help: max requests to list (default 20)."},
    "profile":{"type":"object", "description":"For op=edit/create: agent fields to apply. Keys: slug (create only), name, soul, model, fallbacks (array), task_type, max_cost_mc, max_daily_mc, memory_scope, workdir, description. op=edit applies them wholesale to the target named by \"agent\"."}
  }
}`),
	}
}

type input struct {
	Op      string          `json:"op"`
	Agent   string          `json:"agent"`
	Run     string          `json:"run"`
	Reason  string          `json:"reason"`
	Limit   int             `json:"limit"`
	Profile json.RawMessage `json:"profile"`
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
		p, err := s.SetAgentRetired(strings.TrimSpace(in.Agent), true)
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
		p, err := s.SetAgentRetired(strings.TrimSpace(in.Agent), false)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "retired": false, "action": "revived"}), nil

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
		prof, perr := parseProfile(in.Profile)
		if perr != nil {
			return errResult(perr.Error()), nil
		}
		p, err := s.EditAgent(ref, prof)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"agent": p.Slug, "action": "edited", "profile": agentView(p)}), nil

	case "create":
		prof, perr := parseProfile(in.Profile)
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

	case "":
		return errResult("op required (status|agents|runs|help|cancel|halt|resume|pause|unpause|retire|revive|impact|edit|create)"), nil
	default:
		return errResult("unknown op " + op), nil
	}
}

// parseProfile decodes the op=edit/create "profile" object into a roster.Profile.
// A missing/empty object is an error so a guardian can't silently no-op an edit.
func parseProfile(raw json.RawMessage) (roster.Profile, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return roster.Profile{}, fmt.Errorf(`a "profile" object is required`)
	}
	var p roster.Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return roster.Profile{}, fmt.Errorf("profile: %w", err)
	}
	return p, nil
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
