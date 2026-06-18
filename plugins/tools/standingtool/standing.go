// SPDX-License-Identifier: MIT

// Package standingtool is the in-process standing-order tool: it lets an agent
// create durable trigger rules — "when event X happens, wake the bound agent
// with task Z" or "on this cron schedule, do Z" — plus list and remove them
// (M645).
//
// A standing order is not an agent identity. It is a durable event/cron trigger
// that wakes the bound agent later through the full governed loop. Operators see
// and govern these rules in the Standing cockpit (pause/remove).
package standingtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)

// host is the kernel surface the tool needs — journaled standing CRUD. An
// interface so tests inject a fake (or a real store adapter) without a kernel.
type host interface {
	AddStanding(o standing.Order) (standing.Order, error)
	RemoveStanding(id string) (bool, error)
	Standing() *standing.Store
}

type rosterHost interface {
	Roster() *roster.Store
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the kernel.
type Tool struct {
	host host
}

// New returns an unbound standing tool.
func New() *Tool { return &Tool{} }

// Bind wires the live kernel standing surface. Called once after the kernel opens.
func (t *Tool) Bind(h host) {
	if h != nil {
		t.host = h
	}
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "standing",
		Description: "Create durable event/cron wake rules for this agent. " +
			"A standing order is not an agent identity: it binds event/cron triggers to the bound " +
			"agent's governed task plan. op=create_event fires the plan whenever a matching journal " +
			"event is published (trigger on a subject like \"task.failed\"); op=create_cron fires on " +
			"a cron schedule. op=list / op=remove manage them. Managed sub-agents cannot create " +
			"independently firing self-wake orders; their parent/owner should schedule or run them. " +
			"Use this to set up reactive or recurring behaviour that should happen without the user asking again.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"create, list, or remove autonomous standing orders",
				"created orders may launch future governed agent runs on cron or event triggers",
			},
			AffectedResources: []string{"standing order store", "future autonomous run triggers"},
			RollbackNotes:     "Created standing orders can be paused or removed by id through this tool or the operator cockpit.",
			Confidence:        0.85,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":       {"type":"string", "enum":["create_event","create_cron","list","remove"]},
    "name":     {"type":"string", "description":"A short name for the standing order."},
    "plan":     {"type":"string", "description":"The intent/task to run when the order fires."},
    "subject":  {"type":"string", "description":"For create_event: the journal event subject to trigger on (e.g. \"task.failed\", \"observer.delta\")."},
    "schedule": {"type":"string", "description":"For create_cron: a 5-field cron expression (e.g. \"0 9 * * *\")."},
    "mode":     {"type":"string", "enum":["inform_only","ask","act_or_ask"], "description":"Autonomy when it fires. Default ask (confirm before acting)."},
    "assure":   {"type":"integer", "description":"Optional do-it-for-sure budget: if > 0, each firing runs, verifies it was actually completed, and retries the gap up to this many attempts. Use it for orders whose task must definitely get done."},
    "cooldown_sec": {"type":"integer", "description":"Optional minimum seconds between event-triggered firings. Use this to prevent event storms from waking the order repeatedly."},
    "id":       {"type":"string", "description":"For remove: the standing order id."}
  }
}`),
	}
}

type input struct {
	Op       string `json:"op"`
	Name     string `json:"name"`
	Plan     string `json:"plan"`
	Subject  string `json:"subject"`
	Schedule string `json:"schedule"`
	Mode     string `json:"mode"`
	Assure   int    `json:"assure"`
	Cooldown int64  `json:"cooldown_sec"`
	ID       string `json:"id"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("standing: parse input: %w", err)
	}
	if t.host == nil {
		return errResult("standing orders are not available on this daemon"), nil
	}

	switch in.Op {
	case "create_event":
		if strings.TrimSpace(in.Subject) == "" {
			return errResult(`op=create_event needs a "subject" (the event to trigger on)`), nil
		}
		return t.create(ctx, in, standing.Trigger{Type: standing.TriggerEvent, Subject: strings.TrimSpace(in.Subject)})

	case "create_cron":
		if strings.TrimSpace(in.Schedule) == "" {
			return errResult(`op=create_cron needs a "schedule" (a cron expression)`), nil
		}
		return t.create(ctx, in, standing.Trigger{Type: standing.TriggerCron, Schedule: strings.TrimSpace(in.Schedule)})

	case "remove":
		if in.ID == "" {
			return errResult(`op=remove needs an "id"`), nil
		}
		removed, err := t.host.RemoveStanding(in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !removed {
			return errResult("no standing order with id " + in.ID), nil
		}
		return okJSON(map[string]any{"removed": in.ID}), nil

	case "list":
		orders := t.host.Standing().List()
		out := make([]map[string]any, 0, len(orders))
		for _, o := range orders {
			out = append(out, orderView(o))
		}
		return okJSON(map[string]any{"count": len(out), "orders": out}), nil

	case "":
		return errResult("op required (create_event|create_cron|list|remove)"), nil
	default:
		return errResult("unknown op " + in.Op), nil
	}
}

func (t *Tool) create(ctx context.Context, in input, trig standing.Trigger) (agent.Result, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errResult(`a "name" is required`), nil
	}
	if strings.TrimSpace(in.Plan) == "" {
		return errResult(`a "plan" is required (what to run when it fires)`), nil
	}
	mode := standing.InitiativeMode(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = standing.InitiativeAsk // conservative default: confirm before acting
	}
	assure := max(in.Assure, 0)
	cooldown := max64(in.Cooldown, 0)
	o := standing.Order{
		Name:        name,
		Enabled:     true,
		Triggers:    []standing.Trigger{trig},
		Initiative:  standing.Initiative{Mode: mode},
		Plan:        strings.TrimSpace(in.Plan),
		Assure:      assure,
		CooldownSec: cooldown,
	}
	if actor := strings.TrimSpace(agent.AgentFromContext(ctx)); actor != "" {
		if res, ok := t.validateActingAgent(actor); ok {
			return res, nil
		}
		o.Agent = actor
	}
	saved, err := t.host.AddStanding(o)
	if err != nil {
		return errResult(err.Error()), nil
	}
	v := orderView(saved)
	v["message"] = "standing order created"
	return okJSON(v), nil
}

func (t *Tool) validateActingAgent(slug string) (agent.Result, bool) {
	h, ok := t.host.(rosterHost)
	if !ok || h.Roster() == nil {
		return agent.Result{}, false
	}
	p, found := h.Roster().Get(slug)
	if !found {
		return errResult("acting agent " + slug + " is not in the roster"), true
	}
	if p.Retired {
		return errResult("agent " + p.Slug + " is retired and cannot create a direct standing wake"), true
	}
	if !p.Enabled {
		return errResult("agent " + p.Slug + " is paused and cannot create a direct standing wake"), true
	}
	if !p.AllowsDirectCall() {
		return errResult(managedSubAgentStandingHint(p)), true
	}
	return agent.Result{}, false
}

func managedSubAgentStandingHint(p roster.Profile) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot create a standing order that wakes itself directly; " + hint
}

func orderView(o standing.Order) map[string]any {
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
	if o.CooldownSec > 0 {
		v["cooldown_sec"] = o.CooldownSec
	}
	if strings.TrimSpace(o.Agent) != "" {
		v["agent"] = o.Agent
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
	return agent.Result{Output: "standing: " + msg, IsError: true}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
