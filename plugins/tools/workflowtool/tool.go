// SPDX-License-Identifier: MIT

// Package workflowtool is the in-process `workflow` tool (M802): agents
// author and run durable workflows themselves. An agent composes the graph
// JSON (it knows the node library from the schema below), saves it, runs it
// on demand, and arms/disarms its triggers. Everything lands in the SAME
// store the console canvas and `agt workflow` edit — a workflow an agent
// wrote is a workflow the operator can see, tweak, and kill. Mutating ops
// are gated by the `workflow.manage` Edict capability; list/show are
// introspection. Inside a run every tool node passes the regular policy
// gate, so an agent cannot launder a forbidden call through a workflow.
package workflowtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
)

// Kernel is the slice of the runtime kernel this tool drives — satisfied by
// *runtime.Kernel and easy to fake in tests.
type Kernel interface {
	Workflows() *workflow.Store
	SaveWorkflow(corr string, w workflow.Workflow) (workflow.Workflow, bool, error)
	SetWorkflowEnabled(corr, ref string, enabled bool) (workflow.Workflow, error)
	RunWorkflow(ctx context.Context, corr, ref string, payload any) (runtime.RunWorkflowResult, error)
}

// Tool implements agent.Tool. Construct with New, then Bind the live kernel
// once it opens (the daemon is the single wiring point).
type Tool struct {
	mu sync.RWMutex
	k  Kernel
}

// New returns an unbound Tool — Invoke reports workflows unavailable until
// Bind is called.
func New() *Tool { return &Tool{} }

// Bind wires the live kernel. Called once after the kernel opens.
func (t *Tool) Bind(k Kernel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.k = k
}

func (t *Tool) current() Kernel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.k
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "workflow",
		Description: "Author and run durable workflows — typed-node graphs (trigger/tool/llm/condition/transform/delay/http/code/map/filter/switch/merge/approval/subworkflow) that can start manually, on a cron, or on a journal event. " +
			"Workflows are reusable chains, not agent identities; users, agents, schedules, and webhooks can run the same saved graph. " +
			"op=save validates and stores a whole graph (upsert by name; new workflows arrive DISABLED); " +
			"op=run executes one now (payload becomes {{trigger.payload}}) and returns per-node outputs; " +
			"op=enable arms/disarms its triggers; op=list and op=show inspect the library. " +
			"The graph JSON: {\"name\":..., \"description\":..., \"nodes\":[{\"id\",\"type\",\"label\",\"config\"}], \"edges\":[{\"from\",\"to\",\"port\"}]} — exactly one trigger node, acyclic, condition ports true/false, failable nodes may wire port \"error\". " +
			"Data flows with {{trigger.payload}} / {{<node_id>.output.<path>}} templates. " +
			"Use this to turn a repeatable multi-step job into durable, schedulable automation instead of redoing it by hand every run.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":       {"type":"string", "enum":["save","run","enable","list","show"]},
    "workflow": {"type":"object", "description":"For op=save: the full workflow graph (name, description, nodes, edges)."},
    "ref":      {"type":"string", "description":"For op=run/enable/show: the workflow's id or name."},
    "payload":  {"type":"object", "description":"For op=run (optional): the start payload, readable as {{trigger.payload}}."},
    "enabled":  {"type":"boolean", "description":"For op=enable: true arms the workflow's triggers, false disarms them."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Save, inspect, enable, disable, or run durable workflow graphs.",
				"Enabled workflows can launch future governed runs; manual runs may execute each workflow node's own effects.",
			},
			AffectedResources: []string{"workflow store", "workflow trigger state", "tools invoked by workflow runs"},
			RollbackNotes:     "Disable or edit stored workflows to revert automation; compensate already-executed workflow node effects according to each node/tool.",
			Confidence:        0.7,
		},
	}
}

type input struct {
	Op       string          `json:"op"`
	Workflow json.RawMessage `json:"workflow"`
	Ref      string          `json:"ref"`
	Payload  json.RawMessage `json:"payload"`
	Enabled  *bool           `json:"enabled"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("workflow: parse input: %w", err)
	}
	k := t.current()
	if k == nil {
		return errResult("workflows are not available on this daemon"), nil
	}
	corr := agent.CorrelationFromContext(ctx)

	switch in.Op {
	case "save":
		if len(in.Workflow) == 0 {
			return errResult(`op=save needs a "workflow" object (name, nodes, edges)`), nil
		}
		var w workflow.Workflow
		if err := json.Unmarshal(in.Workflow, &w); err != nil {
			return errResult("workflow JSON: " + err.Error()), nil
		}
		saved, created, err := k.SaveWorkflow(corr, w)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if created && saved.Enabled {
			// Console saves arrive armed (an operator clicked Save); AGENT
			// saves arrive disabled — installing standing automation takes an
			// explicit (gated) op=enable, never rides along with the save.
			if disarmed, derr := k.SetWorkflowEnabled(corr, saved.Name, false); derr == nil {
				saved = disarmed
			}
		}
		v := view(saved)
		if created {
			v["message"] = "created (disabled) — run it with op=run; op=enable arms its trigger"
		} else {
			v["message"] = "updated"
		}
		return okJSON(v), nil

	case "run":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=run needs a "ref" (the workflow's id or name)`), nil
		}
		var payload any
		if len(in.Payload) > 0 {
			if err := json.Unmarshal(in.Payload, &payload); err != nil {
				return errResult("payload JSON: " + err.Error()), nil
			}
		}
		res, err := k.RunWorkflow(ctx, corr, strings.TrimSpace(in.Ref), payload)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"executed": res.Executed,
			"outputs":  res.Outputs,
		}), nil

	case "enable":
		if strings.TrimSpace(in.Ref) == "" || in.Enabled == nil {
			return errResult(`op=enable needs a "ref" and an "enabled" boolean`), nil
		}
		w, err := k.SetWorkflowEnabled(corr, strings.TrimSpace(in.Ref), *in.Enabled)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(view(w)), nil

	case "list":
		all := k.Workflows().List()
		views := make([]map[string]any, 0, len(all))
		for _, w := range all {
			views = append(views, view(w))
		}
		return okJSON(map[string]any{"count": len(views), "workflows": views}), nil

	case "show":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=show needs a "ref" (the workflow's id or name)`), nil
		}
		w, found := k.Workflows().Get(strings.TrimSpace(in.Ref))
		if !found {
			return errResult("no workflow " + in.Ref), nil
		}
		v := view(w)
		v["nodes"] = w.Nodes
		v["edges"] = w.Edges
		return okJSON(v), nil

	case "":
		return errResult("op required (save|run|enable|list|show)"), nil
	default:
		return errResult("unknown op " + in.Op + " (save|run|enable|list|show)"), nil
	}
}

func view(w workflow.Workflow) map[string]any {
	v := map[string]any{
		"id":      w.ID,
		"name":    w.Name,
		"enabled": w.Enabled,
		"nodes":   len(w.Nodes),
		"edges":   len(w.Edges),
	}
	if w.Description != "" {
		v["description"] = w.Description
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
	return agent.Result{Output: "workflow: " + msg, IsError: true}
}
