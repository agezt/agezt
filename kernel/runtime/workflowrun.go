// SPDX-License-Identifier: MIT

package runtime

// Workflow engine integration (M798): the kernel-side CRUD + execution of
// kernel/workflow graphs. Execution is token-flow over the validated DAG:
// the trigger fires, each completing node fires its outgoing edges (a
// condition fires exactly one port), and a node runs once on its first
// incoming token. Tool nodes pass the SAME Edict policy hook as agent-loop
// tool calls (deny/ask semantics identical), llm nodes ride the Governor
// (budgets/routing/cost metering), and every step is journaled
// (workflow.*) so the console canvas can replay a run live.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/workflow"
)

// Workflows returns the durable workflow store (M798). Always non-nil after
// Open.
func (k *Kernel) Workflows() *workflow.Store { return k.workflows }

// SaveWorkflow validates and upserts a workflow (the canvas posts the whole
// graph), journaling workflow.saved with created/updated.
func (k *Kernel) SaveWorkflow(corr string, w workflow.Workflow) (workflow.Workflow, bool, error) {
	saved, created, err := k.workflows.Save(w)
	if err != nil {
		return workflow.Workflow{}, false, err
	}
	action := "updated"
	if created {
		action = "created"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + saved.Name, Kind: event.KindWorkflowSaved, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "action": action, "nodes": len(saved.Nodes), "edges": len(saved.Edges)},
	})
	return saved, created, nil
}

// SetWorkflowEnabled arms/disarms a workflow's triggers, journaling
// workflow.updated.
func (k *Kernel) SetWorkflowEnabled(corr, ref string, enabled bool) (workflow.Workflow, error) {
	w, err := k.workflows.SetEnabled(ref, enabled)
	if err != nil {
		return workflow.Workflow{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowUpdated, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": w.ID, "name": w.Name, "enabled": enabled},
	})
	return w, nil
}

// RemoveWorkflow deletes a workflow, journaling workflow.removed when it
// existed.
func (k *Kernel) RemoveWorkflow(corr, ref string) (bool, error) {
	gone, ok, err := k.workflows.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + gone.Name, Kind: event.KindWorkflowRemoved, Actor: "workflow",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// workflowStepCap is a defense-in-depth bound on executed steps per run —
// validation already rejects cycles, so this can only fire on an engine bug.
const workflowStepCap = 256

// RunWorkflowResult carries one run's outcome: per-node outputs (by node id)
// and the ordered list of executed node ids.
type RunWorkflowResult struct {
	Outputs  map[string]any
	Executed []string
}

// RunWorkflow executes one stored workflow under corr. payload becomes
// {{trigger.payload}}. Halted kernels refuse; the run respects ctx
// cancellation between and inside nodes. The first failing node fails the
// run (error branching arrives with M800).
func (k *Kernel) RunWorkflow(ctx context.Context, corr, ref string, payload any) (RunWorkflowResult, error) {
	k.mu.Lock()
	halted := k.halted
	k.mu.Unlock()
	if halted {
		return RunWorkflowResult{}, ErrHalted
	}
	w, found := k.workflows.Get(ref)
	if !found {
		return RunWorkflowResult{}, workflow.ErrNotFound
	}
	if err := workflow.Validate(w); err != nil { // defense: stores can predate rules
		return RunWorkflowResult{}, err
	}

	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowStarted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": w.ID, "name": w.Name, "nodes": len(w.Nodes)},
	})

	res, err := k.runWorkflowGraph(ctx, corr, w, payload)
	if err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowFailed, Actor: "workflow",
			CorrelationID: corr,
			Payload:       map[string]any{"id": w.ID, "name": w.Name, "error": err.Error(), "executed": res.Executed},
		})
		return res, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowCompleted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       map[string]any{"id": w.ID, "name": w.Name, "executed": res.Executed},
	})
	return res, nil
}

func (k *Kernel) runWorkflowGraph(ctx context.Context, corr string, w workflow.Workflow, payload any) (RunWorkflowResult, error) {
	// Outgoing edges grouped by (from, port).
	edges := map[string]map[string][]string{}
	for _, e := range w.Edges {
		if edges[e.From] == nil {
			edges[e.From] = map[string][]string{}
		}
		edges[e.From][e.Port] = append(edges[e.From][e.Port], e.To)
	}

	data := map[string]any{"trigger": map[string]any{"payload": payload}}
	res := RunWorkflowResult{Outputs: map[string]any{}}
	executed := map[string]bool{}

	queue := []string{w.TriggerNode().ID}
	steps := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		steps++
		if steps > workflowStepCap {
			return res, errors.New("workflow: step cap exceeded (engine bug — the graph validated as acyclic)")
		}
		id := queue[0]
		queue = queue[1:]
		if executed[id] {
			continue // a node runs once: first token wins
		}
		executed[id] = true
		node := w.NodeByID(id)

		output, port, err := k.execWorkflowNode(ctx, corr, node, data, payload)
		nodePayload := map[string]any{
			"workflow": w.Name, "node": id, "type": node.Type, "ok": err == nil,
		}
		if node.Label != "" {
			nodePayload["label"] = node.Label
		}
		if port != "" {
			nodePayload["port"] = port
		}
		if err != nil {
			nodePayload["error"] = err.Error()
		}
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowNode, Actor: "workflow",
			CorrelationID: corr,
			Payload:       nodePayload,
		})
		if err != nil {
			return res, fmt.Errorf("node %s: %w", id, err)
		}

		data[id] = map[string]any{"output": output}
		res.Outputs[id] = output
		res.Executed = append(res.Executed, id)
		queue = append(queue, edges[id][port]...)
	}
	return res, nil
}

// execWorkflowNode runs one node and returns (output, firedPort, error).
// Every node except condition fires the default "" port.
func (k *Kernel) execWorkflowNode(ctx context.Context, corr string, n *workflow.Node, data map[string]any, payload any) (any, string, error) {
	switch n.Type {
	case workflow.NodeTrigger:
		return payload, "", nil

	case workflow.NodeTool:
		var c workflow.ToolConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		args := strings.TrimSpace(workflow.Interpolate(string(c.Args), data))
		if args == "" {
			args = "{}"
		}
		// The exact policy gate agent-loop tool calls pass: deny refuses the
		// node, ask blocks on the operator via the approval registry.
		verdict := k.policyHook(ctx, agent.ToolCall{ID: "wf-" + n.ID, Name: c.Tool, Input: json.RawMessage(args)})
		if !verdict.Allow {
			reason := verdict.Reason
			if reason == "" {
				reason = "denied by policy"
			}
			return nil, "", fmt.Errorf("tool %s refused: %s", c.Tool, reason)
		}
		tools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
		tool, ok := tools[c.Tool]
		if !ok {
			return nil, "", fmt.Errorf("unknown tool %q", c.Tool)
		}
		out, err := tool.Invoke(ctx, json.RawMessage(args))
		if err != nil {
			return nil, "", err
		}
		if out.IsError {
			return nil, "", fmt.Errorf("tool %s failed: %s", c.Tool, truncateForErr(out.Output))
		}
		return parseMaybeJSON(out.Output), "", nil

	case workflow.NodeLLM:
		var c workflow.LLMConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		model := strings.TrimSpace(c.Model)
		if model == "" {
			model = k.cfg.Model
		}
		resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
			Model:    model,
			System:   workflow.Interpolate(c.System, data),
			TaskType: "workflow", // route + meter workflow completions as their own class
			Messages: []agent.Message{{Role: agent.RoleUser, Content: workflow.Interpolate(c.Prompt, data)}},
		})
		if err != nil {
			return nil, "", err
		}
		return strings.TrimSpace(resp.Message.Content), "", nil

	case workflow.NodeCondition:
		var c workflow.ConditionConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		left := workflow.Interpolate(c.Left, data)
		right := workflow.Interpolate(c.Right, data)
		truth, err := evalCondition(left, c.Op, right)
		if err != nil {
			return nil, "", err
		}
		port := "false"
		if truth {
			port = "true"
		}
		return truth, port, nil

	case workflow.NodeTransform:
		var c workflow.TransformConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		return parseMaybeJSON(workflow.Interpolate(c.Template, data)), "", nil

	case workflow.NodeDelay:
		var c workflow.DelayConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		select {
		case <-time.After(time.Duration(c.Seconds * float64(time.Second))):
			return c.Seconds, "", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}

	default:
		return nil, "", fmt.Errorf("unknown node type %q", n.Type)
	}
}

func evalCondition(left, op, right string) (bool, error) {
	switch op {
	case "equals":
		return left == right, nil
	case "not_equals":
		return left != right, nil
	case "contains":
		return strings.Contains(left, right), nil
	case "not_empty":
		return strings.TrimSpace(left) != "", nil
	case "empty":
		return strings.TrimSpace(left) == "", nil
	case "gt", "lt":
		l, lerr := strconv.ParseFloat(strings.TrimSpace(left), 64)
		r, rerr := strconv.ParseFloat(strings.TrimSpace(right), 64)
		if lerr != nil || rerr != nil {
			return false, fmt.Errorf("condition %s needs numbers, got %q / %q", op, left, right)
		}
		if op == "gt" {
			return l > r, nil
		}
		return l < r, nil
	default:
		return false, fmt.Errorf("unknown condition op %q", op)
	}
}

// parseMaybeJSON keeps structured outputs structured: a tool/transform whose
// text parses as JSON becomes the parsed value (so {{node.output.field}}
// works downstream); anything else stays a string.
func parseMaybeJSON(s string) any {
	t := strings.TrimSpace(s)
	if len(t) > 0 && (t[0] == '{' || t[0] == '[') {
		var v any
		if err := json.Unmarshal([]byte(t), &v); err == nil {
			return v
		}
	}
	return s
}

func truncateForErr(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
