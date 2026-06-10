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
	"github.com/agezt/agezt/kernel/approval"
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
	// Outgoing edges grouped by (from, port); incoming counts for merge "all".
	edges := map[string]map[string][]string{}
	indegree := map[string]int{}
	for _, e := range w.Edges {
		if edges[e.From] == nil {
			edges[e.From] = map[string][]string{}
		}
		edges[e.From][e.Port] = append(edges[e.From][e.Port], e.To)
		indegree[e.To]++
	}

	data := map[string]any{"trigger": map[string]any{"payload": payload}}
	res := RunWorkflowResult{Outputs: map[string]any{}}
	executed := map[string]bool{}
	tokens := map[string]int{} // incoming tokens received, for merge mode "all"

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
		node := w.NodeByID(id)
		// A merge in "all" mode waits for a token on EVERY incoming edge —
		// later tokens re-enqueue it, so skipping here is safe.
		if node.Type == workflow.NodeMerge && mergeMode(node) == "all" && tokens[id] < indegree[id] {
			continue
		}
		executed[id] = true

		output, port, err := k.execWorkflowNode(ctx, corr, node, w, data, payload)
		handled := false
		if err != nil && len(edges[id]["error"]) > 0 {
			// The node wired an error branch: the run survives, the error
			// message becomes the node's output, and the error port fires.
			output, port, handled = map[string]any{"error": err.Error()}, "error", true
		}
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
			nodePayload["handled"] = handled
		}
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowNode, Actor: "workflow",
			CorrelationID: corr,
			Payload:       nodePayload,
		})
		if err != nil && !handled {
			return res, fmt.Errorf("node %s: %w", id, err)
		}

		data[id] = map[string]any{"output": output}
		res.Outputs[id] = output
		res.Executed = append(res.Executed, id)
		for _, next := range edges[id][port] {
			tokens[next]++
			queue = append(queue, next)
		}
	}
	return res, nil
}

func mergeMode(n *workflow.Node) string {
	var c workflow.MergeConfig
	_ = json.Unmarshal(n.Config, &c)
	if c.Mode == "" {
		return "any"
	}
	return c.Mode
}

// wfDepthKey carries subworkflow nesting depth through the run context.
type wfDepthKey struct{}

const maxSubflowDepth = 3

// execWorkflowNode runs one node and returns (output, firedPort, error).
// Most nodes fire the default "" port; condition fires true/false, switch
// fires a case port or "default".
func (k *Kernel) execWorkflowNode(ctx context.Context, corr string, n *workflow.Node, w workflow.Workflow, data map[string]any, payload any) (any, string, error) {
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
		return k.invokeWorkflowTool(ctx, c.Tool, "wf-"+n.ID, json.RawMessage(args))

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

	case workflow.NodeHTTP:
		var c workflow.HTTPConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		// Ride the registered http tool — its host allowlist / egress guard
		// and the CapHTTPGet/Post policy mapping apply exactly as they would
		// to an agent's own call.
		headers := make(map[string]string, len(c.Headers))
		for hk, hv := range c.Headers {
			headers[hk] = workflow.Interpolate(hv, data)
		}
		args, err := json.Marshal(map[string]any{
			"method":       strings.ToUpper(strings.TrimSpace(c.Method)),
			"url":          workflow.Interpolate(c.URL, data),
			"headers":      headers,
			"body":         workflow.Interpolate(c.Body, data),
			"content_type": c.ContentType,
		})
		if err != nil {
			return nil, "", err
		}
		return k.invokeWorkflowTool(ctx, "http", "wf-"+n.ID, args)

	case workflow.NodeCode:
		var c workflow.CodeConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		if k.cfg.ScriptRunner == nil {
			return nil, "", errors.New("code nodes need the code-exec sandbox (not available on this daemon)")
		}
		// The same code.exec policy gate a direct code_exec call passes.
		probe, _ := json.Marshal(map[string]any{"language": c.Language, "code": c.Code})
		verdict := k.policyHook(ctx, agent.ToolCall{ID: "wf-" + n.ID, Name: "code_exec", Input: probe})
		if !verdict.Allow {
			reason := verdict.Reason
			if reason == "" {
				reason = "denied by policy"
			}
			return nil, "", fmt.Errorf("code refused: %s", reason)
		}
		input := strings.TrimSpace(workflow.Interpolate(c.Input, data))
		if input == "" {
			input = "{}"
		}
		out, isErr, err := k.cfg.ScriptRunner.RunScript(ctx, c.Language, c.Code, input)
		if err != nil {
			return nil, "", err
		}
		if isErr {
			return nil, "", fmt.Errorf("code failed: %s", truncateForErr(out))
		}
		return parseMaybeJSON(out), "", nil

	case workflow.NodeMap:
		var c workflow.MapConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		items, err := workflowItems(c.Items, data)
		if err != nil {
			return nil, "", err
		}
		out := make([]any, 0, len(items))
		for i, item := range items {
			out = append(out, parseMaybeJSONValue(workflow.Interpolate(c.Template, withItem(data, item, i))))
		}
		return out, "", nil

	case workflow.NodeFilter:
		var c workflow.FilterConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		items, err := workflowItems(c.Items, data)
		if err != nil {
			return nil, "", err
		}
		out := make([]any, 0, len(items))
		for i, item := range items {
			itemData := withItem(data, item, i)
			keep, cerr := evalCondition(workflow.Interpolate(c.Left, itemData), c.Op, workflow.Interpolate(c.Right, itemData))
			if cerr != nil {
				return nil, "", cerr
			}
			if keep {
				out = append(out, item)
			}
		}
		return out, "", nil

	case workflow.NodeSwitch:
		var c workflow.SwitchConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		val := workflow.Interpolate(c.Value, data)
		for _, cs := range c.Cases {
			if val == cs.Equals {
				return val, cs.Port, nil
			}
		}
		return val, "default", nil

	case workflow.NodeMerge:
		// Collect the outputs that arrived on incoming edges, in edge order.
		var inputs []any
		for _, e := range w.Edges {
			if e.To != n.ID {
				continue
			}
			if up, ok := data[e.From].(map[string]any); ok {
				inputs = append(inputs, up["output"])
			}
		}
		return inputs, "", nil

	case workflow.NodeApproval:
		var c workflow.ApprovalConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		capability := strings.TrimSpace(c.Capability)
		if capability == "" {
			capability = "workflow.approve"
		}
		desc := workflow.Interpolate(c.Description, data)
		out := k.approvals.Submit(ctx, approval.SubmitSpec{
			Capability:    capability,
			ToolName:      "workflow.approval",
			Input:         desc,
			Reason:        desc,
			Actor:         "workflow",
			CorrelationID: corr,
		})
		if out.Decision != approval.DecisionGrant {
			return nil, "", fmt.Errorf("approval %s: %s", out.Decision, out.Reason)
		}
		return "granted by " + out.ResolvedBy, "", nil

	case workflow.NodeSubflow:
		var c workflow.SubflowConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		depth, _ := ctx.Value(wfDepthKey{}).(int)
		if depth+1 >= maxSubflowDepth {
			return nil, "", fmt.Errorf("subworkflow nesting deeper than %d refused", maxSubflowDepth)
		}
		var subPayload any
		if strings.TrimSpace(c.Payload) != "" {
			subPayload = parseMaybeJSONValue(workflow.Interpolate(c.Payload, data))
		}
		subCtx := context.WithValue(ctx, wfDepthKey{}, depth+1)
		subRes, err := k.RunWorkflow(subCtx, corr, c.Workflow, subPayload)
		if err != nil {
			return nil, "", fmt.Errorf("subworkflow %s: %w", c.Workflow, err)
		}
		return map[string]any{"executed": subRes.Executed, "outputs": subRes.Outputs}, "", nil

	default:
		return nil, "", fmt.Errorf("unknown node type %q", n.Type)
	}
}

// invokeWorkflowTool runs one named tool through the policy gate — shared by
// the tool and http nodes.
func (k *Kernel) invokeWorkflowTool(ctx context.Context, toolName, callID string, args json.RawMessage) (any, string, error) {
	verdict := k.policyHook(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args})
	if !verdict.Allow {
		reason := verdict.Reason
		if reason == "" {
			reason = "denied by policy"
		}
		return nil, "", fmt.Errorf("tool %s refused: %s", toolName, reason)
	}
	tools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
	tool, ok := tools[toolName]
	if !ok {
		return nil, "", fmt.Errorf("unknown tool %q", toolName)
	}
	out, err := tool.Invoke(ctx, args)
	if err != nil {
		return nil, "", err
	}
	if out.IsError {
		return nil, "", fmt.Errorf("tool %s failed: %s", toolName, truncateForErr(out.Output))
	}
	return parseMaybeJSON(out.Output), "", nil
}

// workflowItems resolves a map/filter items reference — "{{a.output.list}}"
// or the bare path "a.output.list" — to the array it names.
func workflowItems(ref string, data map[string]any) ([]any, error) {
	path := strings.TrimSpace(ref)
	path = strings.TrimPrefix(path, "{{")
	path = strings.TrimSuffix(path, "}}")
	v := workflow.Lookup(data, strings.TrimSpace(path))
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("items %q did not resolve to an array", ref)
	}
	const maxItems = 1000
	if len(items) > maxItems {
		return nil, fmt.Errorf("items %q has %d elements (max %d)", ref, len(items), maxItems)
	}
	return items, nil
}

// withItem extends the run context with the current element for per-item
// templates ({{item}}, {{item.field}}, {{index}}).
func withItem(data map[string]any, item any, index int) map[string]any {
	out := make(map[string]any, len(data)+2)
	for dk, dv := range data {
		out[dk] = dv
	}
	out["item"] = item
	out["index"] = index
	return out
}

// parseMaybeJSONValue is parseMaybeJSON for already-trimmed template output.
func parseMaybeJSONValue(s string) any { return parseMaybeJSON(s) }

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
