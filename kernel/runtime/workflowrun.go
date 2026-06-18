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

	"github.com/agezt/agezt/internal/apperrors"
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
	k.runsMu.Lock()
	halted := k.halted
	k.runsMu.Unlock()
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
	runMeta := workflowRunProvenance(ctx)

	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowStarted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "nodes": len(w.Nodes)}, runMeta),
	})

	res, err := k.runWorkflowGraph(ctx, corr, w, payload)
	if err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowFailed, Actor: "workflow",
			CorrelationID: corr,
			Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "error": err.Error(), "executed": res.Executed}, runMeta),
		})
		return res, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowCompleted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeWorkflowRunPayload(map[string]any{"id": w.ID, "name": w.Name, "executed": res.Executed}, runMeta),
	})
	return res, nil
}

func workflowRunProvenance(ctx context.Context) map[string]any {
	wake := wakeContextFromCtx(ctx)
	source := strings.TrimSpace(wake.Source)
	if source == "" {
		source = "manual"
	}
	agentSlug := strings.TrimSpace(agent.AgentFromContext(ctx))
	runner := source
	if agentSlug != "" {
		runner = "agent"
	}
	out := map[string]any{
		"source": source,
		"runner": runner,
	}
	if agentSlug != "" {
		out["agent"] = agentSlug
	}
	if wake.ScheduleID != "" {
		out["schedule_id"] = wake.ScheduleID
	}
	if wake.StandingID != "" {
		out["standing_id"] = wake.StandingID
	}
	if wake.StandingName != "" {
		out["standing_name"] = wake.StandingName
	}
	if wake.TriggerSubject != "" {
		out["trigger_subject"] = wake.TriggerSubject
	}
	if wake.ParentCorrelation != "" {
		out["parent_correlation_id"] = wake.ParentCorrelation
	}
	return out
}

func mergeWorkflowRunPayload(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
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
	head := 0
	steps := 0
	for head < len(queue) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		steps++
		if steps > workflowStepCap {
			return res, errors.New("workflow: step cap exceeded (engine bug — the graph validated as acyclic)")
		}
		id := queue[head]
		head++
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

		inputPreview := nodeInputPreview(node, data)
		output, port, attempts, err := k.execNodeWithReliability(ctx, corr, node, w, data, payload)
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
		if attempts > 1 {
			nodePayload["attempts"] = attempts
		}
		// Inspectability (M808): the exact data the node consumed and
		// produced rides the journal (truncated), so the canvas can show it
		// live AND for any historical run. The journal is the truth.
		if inputPreview != "" {
			nodePayload["input"], _ = wfSnippet(inputPreview)
		}
		if err == nil || handled {
			if snip, truncated := wfSnippet(output); snip != "" {
				nodePayload["output"] = snip
				if truncated {
					nodePayload["output_truncated"] = true
				}
			}
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
			return res, apperrors.WrapSimplef("node %s: %%w", err, id)
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

// execNodeWithReliability (M808) wraps one node's execution with its
// reliability settings: each ATTEMPT gets its own deadline when timeout_sec
// is set, and failable nodes re-run up to retries times (pausing
// retry_delay_sec between attempts). A cancelled RUN never retries — only
// the node's own failures do. Returns the attempts actually used so the
// journal can say "succeeded on attempt 3".
func (k *Kernel) execNodeWithReliability(ctx context.Context, corr string, n *workflow.Node, w workflow.Workflow, data map[string]any, payload any) (any, string, int, error) {
	attempts := 1 + n.Retries
	var (
		output any
		port   string
		err    error
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		actx, cancel := ctx, context.CancelFunc(nil)
		if n.TimeoutSec > 0 {
			actx, cancel = context.WithTimeout(ctx, time.Duration(n.TimeoutSec)*time.Second)
		}
		output, port, err = k.execWorkflowNode(actx, corr, n, w, data, payload)
		if cancel != nil {
			// Name the per-node deadline distinctly: "context deadline
			// exceeded" alone reads as the whole run dying.
			if err != nil && errors.Is(actx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				err = fmt.Errorf("node timeout after %ds: %w", n.TimeoutSec, err)
			}
			cancel()
		}
		if err == nil {
			return output, port, attempt, nil
		}
		if ctx.Err() != nil || attempt == attempts {
			return output, port, attempt, err
		}
		if n.RetryDelaySec > 0 {
			select {
			case <-time.After(time.Duration(n.RetryDelaySec) * time.Second):
			case <-ctx.Done():
				return output, port, attempt, err
			}
		}
	}
	return output, port, attempts, err
}

// wfSnippetMax bounds the per-node data snippet journaled with each
// workflow.node event — enough to inspect, never enough to bloat the chain.
const wfSnippetMax = 2000

// wfSnippet renders a node's data for the journal: strings verbatim,
// everything else compact JSON, truncated at wfSnippetMax runes.
func wfSnippet(v any) (string, bool) {
	var s string
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		s = t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", false
		}
		s = string(b)
	}
	if r := []rune(s); len(r) > wfSnippetMax {
		return string(r[:wfSnippetMax]) + "…", true
	}
	return s, false
}

// nodeInputPreview resolves what a node is ABOUT to consume — the
// interpolated args/prompt/url/items — for the journal's input snippet.
// Preview-only (the executor re-interpolates authoritatively); a node type
// with no meaningful input previews as "".
func nodeInputPreview(n *workflow.Node, data map[string]any) string {
	switch n.Type {
	case workflow.NodeTool:
		var c workflow.ToolConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(workflow.Interpolate(string(c.Args), data))
	case workflow.NodeLLM:
		var c workflow.LLMConfig
		_ = json.Unmarshal(n.Config, &c)
		return workflow.Interpolate(c.Prompt, data)
	case workflow.NodeCondition:
		var c workflow.ConditionConfig
		_ = json.Unmarshal(n.Config, &c)
		return workflow.Interpolate(c.Left, data) + " " + c.Op + " " + workflow.Interpolate(c.Right, data)
	case workflow.NodeHTTP:
		var c workflow.HTTPConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.ToUpper(strings.TrimSpace(c.Method)) + " " + workflow.Interpolate(c.URL, data)
	case workflow.NodeCode:
		var c workflow.CodeConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(workflow.Interpolate(c.Input, data))
	case workflow.NodeMap, workflow.NodeFilter:
		var c struct {
			Items string `json:"items"`
		}
		_ = json.Unmarshal(n.Config, &c)
		return workflow.Interpolate(c.Items, data)
	case workflow.NodeSwitch:
		var c workflow.SwitchConfig
		_ = json.Unmarshal(n.Config, &c)
		return workflow.Interpolate(c.Value, data)
	case workflow.NodeSubflow:
		var c workflow.SubflowConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(workflow.Interpolate(c.Payload, data))
	case workflow.NodePipeline:
		var c workflow.PipelineConfig
		_ = json.Unmarshal(n.Config, &c)
		parts := make([]string, 0, len(c.Steps))
		for _, step := range c.Steps {
			args := strings.TrimSpace(workflow.Interpolate(string(step.Args), data))
			if args == "" {
				args = "{}"
			}
			parts = append(parts, strings.TrimSpace(step.Tool)+"("+args+")")
		}
		return strings.Join(parts, " -> ")
	}
	return ""
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
			if m := modelFromCtx(ctx); m != "" {
				model = m
			} else if m, ok := agentConfigStringOverride(ctx, "AGEZT_MODEL"); ok {
				model = m
			} else {
				model = k.cfg.Model
			}
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
			return nil, "", apperrors.WrapSimplef("subworkflow %s: %%w", err, c.Workflow)
		}
		return map[string]any{"executed": subRes.Executed, "outputs": subRes.Outputs}, "", nil

	case workflow.NodePipeline:
		var c workflow.PipelineConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		return k.execPipelineNode(ctx, n.ID, c, data)

	default:
		return nil, "", fmt.Errorf("unknown node type %q", n.Type)
	}
}

func (k *Kernel) execPipelineNode(ctx context.Context, nodeID string, c workflow.PipelineConfig, data map[string]any) (any, string, error) {
	steps := map[string]any{}
	var last any
	for _, step := range c.Steps {
		stepData := copyWorkflowData(data)
		stepData["steps"] = steps
		args := strings.TrimSpace(workflow.Interpolate(string(step.Args), stepData))
		if args == "" {
			args = "{}"
		}
		out, _, err := k.invokeWorkflowTool(ctx, step.Tool, "wf-"+nodeID+"-"+step.ID, json.RawMessage(args))
		if err != nil {
			return nil, "", fmt.Errorf("pipeline step %s: %w", step.ID, err)
		}
		if len(strings.TrimSpace(string(step.OutputSchema))) > 0 {
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, "", fmt.Errorf("pipeline step %s output cannot be encoded as JSON: %w", step.ID, err)
			}
			def := agent.ToolDef{Name: "pipeline." + nodeID + "." + step.ID + ".output", InputSchema: step.OutputSchema}
			if err := agent.ValidateToolInput(def, raw); err != nil {
				return nil, "", fmt.Errorf("pipeline step %s output rejected by schema: %w", step.ID, err)
			}
		}
		steps[step.ID] = map[string]any{"tool": step.Tool, "output": out}
		last = out
	}
	return map[string]any{"last": last, "steps": steps}, "", nil
}

func copyWorkflowData(data map[string]any) map[string]any {
	out := make(map[string]any, len(data)+1)
	for k, v := range data {
		out[k] = v
	}
	return out
}

// invokeWorkflowTool runs one named tool through the policy gate — shared by
// the tool and http nodes.
func (k *Kernel) invokeWorkflowTool(ctx context.Context, toolName, callID string, args json.RawMessage) (any, string, error) {
	tools := k.mergeMCPTools(k.mergeScriptTools(k.tools))
	tool, ok := tools[toolName]
	if !ok {
		return nil, "", fmt.Errorf("unknown tool %q", toolName)
	}
	if err := agent.ValidateToolInput(tool.Definition(), args); err != nil {
		return nil, "", fmt.Errorf("tool %s input rejected by schema: %w", toolName, err)
	}
	verdict := k.policyHook(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args})
	if !verdict.Allow {
		reason := verdict.Reason
		if reason == "" {
			reason = "denied by policy"
		}
		return nil, "", fmt.Errorf("tool %s refused: %s", toolName, reason)
	}
	out, err := tool.Invoke(ctx, args)
	if err != nil {
		k.completeAgentNoiseNotify(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, agent.Result{Output: err.Error(), IsError: true})
		return nil, "", err
	}
	if out.IsError {
		k.completeAgentNoiseNotify(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, out)
		return nil, "", fmt.Errorf("tool %s failed: %s", toolName, truncateForErr(out.Output))
	}
	k.completeAgentNoiseNotify(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, out)
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
