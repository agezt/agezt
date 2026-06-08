// SPDX-License-Identifier: MIT

// Package standingtool is the in-process standing-order tool: it lets the agent
// create its OWN autonomous, trigger-driven agents (Chronos standing orders) —
// "when event X happens, run plan Y" or "on this cron schedule, do Z" — plus
// list and remove them (M645).
//
// This is the agents-create-agents primitive for the EVENT/cron axis, symmetric
// with the `schedule` tool (which covers the time axis). A standing order the
// agent creates fires later through the full governed loop; the operator sees
// and governs them in the Standing cockpit (pause/remove). Tagged via name so
// it's clear which were set up autonomously.
package standingtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/standing"
)

// host is the kernel surface the tool needs — journaled standing CRUD. An
// interface so tests inject a fake (or a real store adapter) without a kernel.
type host interface {
	AddStanding(o standing.Order) (standing.Order, error)
	RemoveStanding(id string) (bool, error)
	Standing() *standing.Store
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
		Description: "Create your OWN autonomous, trigger-driven agents (standing orders): " +
			"op=create_event makes one that fires its plan whenever a matching journal event " +
			"is published (trigger on a subject like \"task.failed\"); op=create_cron makes one " +
			"that fires on a cron schedule. op=list / op=remove manage them. A standing order " +
			"runs its plan later through the full governed loop. Use this to set up reactive or " +
			"recurring behaviour that should happen without the user asking again.",
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
	ID       string `json:"id"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
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
		return t.create(in, standing.Trigger{Type: standing.TriggerEvent, Subject: strings.TrimSpace(in.Subject)})

	case "create_cron":
		if strings.TrimSpace(in.Schedule) == "" {
			return errResult(`op=create_cron needs a "schedule" (a cron expression)`), nil
		}
		return t.create(in, standing.Trigger{Type: standing.TriggerCron, Schedule: strings.TrimSpace(in.Schedule)})

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

func (t *Tool) create(in input, trig standing.Trigger) (agent.Result, error) {
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
	o := standing.Order{
		Name:       name,
		Enabled:    true,
		Triggers:   []standing.Trigger{trig},
		Initiative: standing.Initiative{Mode: mode},
		Plan:       strings.TrimSpace(in.Plan),
	}
	saved, err := t.host.AddStanding(o)
	if err != nil {
		return errResult(err.Error()), nil
	}
	v := orderView(saved)
	v["message"] = "standing order created"
	return okJSON(v), nil
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
	return map[string]any{
		"id": o.ID, "name": o.Name, "enabled": o.Enabled,
		"mode": string(o.Initiative.Mode), "triggers": trigs, "plan": o.Plan,
	}
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
