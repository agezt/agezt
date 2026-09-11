// SPDX-License-Identifier: MIT

// Per-node dispatchers: execWorkflowNode + execPipelineNode + copyWorkflowData.
// Code extracted from workflowrun.go during the Day-47 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/workflow"
)


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
			// Same ladder every other run path uses. This used to end at
			// k.cfg.Model — the BOOT model — so after a provider reload
			// hot-swapped the default (M816), an LLM node kept requesting the
			// model the daemon started with, usually the one just de-keyed.
			model, _ = resolveRunModel(ctx, k.effectiveConfig(ctx))
		}
		// completeAux stamps CorrelationID (previously dropped here, leaving
		// llm-node spend unattributable) alongside the workflow routing class.
		resp, err := k.completeAux(ctx, corr, "workflow", agent.CompletionRequest{
			Model:    model,
			System:   workflow.Interpolate(c.System, data),
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