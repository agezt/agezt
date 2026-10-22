// SPDX-License-Identifier: MIT

package workflow

// The per-node dispatchers: what one node does, and the helpers the node
// types share. Every effect leaves through the host's ports — a tool call is
// a governed tool call, an llm node is a metered completion, a code node is
// a sandboxed script.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/ulid"
)

// wfDepthKey carries subworkflow nesting depth through the run context.
type wfDepthKey struct{}

const maxSubflowDepth = 3

func (e *Engine) execWorkflowNode(ctx context.Context, corr string, n *Node, w Workflow, data map[string]any, payload any, prov Provenance) (any, string, error) {
	switch n.Type {
	case NodeTrigger:
		return payload, "", nil

	case NodeTool:
		var c ToolConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		args := strings.TrimSpace(Interpolate(string(c.Args), data))
		if args == "" {
			args = "{}"
		}
		// The exact policy gate agent-loop tool calls pass: deny refuses the
		// node, ask blocks on the operator via the approval registry.
		return e.invokeWorkflowTool(ctx, corr, c.Tool, "wf-"+n.ID, json.RawMessage(args))

	case NodeLLM:
		var c LLMConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		model := strings.TrimSpace(c.Model)
		if model == "" {
			// Same ladder every other run path uses. This used to end at
			// the BOOT model — the daemon's boot model — so after a provider
			// reload hot-swapped the default (M816), an LLM node kept
			// requesting the model the daemon started with, usually the one
			// just de-keyed.
			model = e.model(ctx)
		}
		// completeAux stamps CorrelationID (previously dropped here, leaving
		// llm-node spend unattributable) alongside the workflow routing class.
		resp, err := e.complete(ctx, corr, "workflow", llm.CompletionRequest{
			Model:    model,
			System:   Interpolate(c.System, data),
			Messages: []llm.Message{{Role: llm.RoleUser, Content: Interpolate(c.Prompt, data)}},
		})
		if err != nil {
			return nil, "", err
		}
		return strings.TrimSpace(resp.Message.Content), "", nil

	case NodeCondition:
		var c ConditionConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		left := Interpolate(c.Left, data)
		right := Interpolate(c.Right, data)
		truth, err := evalCondition(left, c.Op, right)
		if err != nil {
			return nil, "", err
		}
		port := "false"
		if truth {
			port = "true"
		}
		return truth, port, nil

	case NodeTransform:
		var c TransformConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		return parseMaybeJSON(Interpolate(c.Template, data)), "", nil

	case NodeDelay:
		var c DelayConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		select {
		case <-time.After(time.Duration(c.Seconds * float64(time.Second))):
			return c.Seconds, "", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}

	case NodeHTTP:
		var c HTTPConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		// Ride the registered http tool — its host allowlist / egress guard
		// and the CapHTTPGet/Post policy mapping apply exactly as they would
		// to an agent's own call.
		headers := make(map[string]string, len(c.Headers))
		for hk, hv := range c.Headers {
			headers[hk] = Interpolate(hv, data)
		}
		args, err := json.Marshal(map[string]any{
			"method":       strings.ToUpper(strings.TrimSpace(c.Method)),
			"url":          Interpolate(c.URL, data),
			"headers":      headers,
			"body":         Interpolate(c.Body, data),
			"content_type": c.ContentType,
		})
		if err != nil {
			return nil, "", err
		}
		return e.invokeWorkflowTool(ctx, corr, "http", "wf-"+n.ID, args)

	case NodeCode:
		var c CodeConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		if !e.scriptRunnerReady() {
			return nil, "", errors.New("code nodes need the code-exec sandbox (not available on this daemon)")
		}
		input := strings.TrimSpace(Interpolate(c.Input, data))
		if input == "" {
			input = "{}"
		}
		res, err := e.runCode(ctx, corr, "wf-"+n.ID, c.Language, c.Code, input)
		if err != nil {
			return nil, "", err
		}
		if res.IsError {
			return nil, "", fmt.Errorf("code failed: %s", truncateForErr(res.Output))
		}
		return parseMaybeJSON(res.Output), "", nil

	case NodeMap:
		var c MapConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		items, err := workflowItems(c.Items, data)
		if err != nil {
			return nil, "", err
		}
		out := make([]any, 0, len(items))
		for i, item := range items {
			out = append(out, parseMaybeJSONValue(Interpolate(c.Template, withItem(data, item, i))))
		}
		return out, "", nil

	case NodeFilter:
		var c FilterConfig
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
			keep, cerr := evalCondition(Interpolate(c.Left, itemData), c.Op, Interpolate(c.Right, itemData))
			if cerr != nil {
				return nil, "", cerr
			}
			if keep {
				out = append(out, item)
			}
		}
		return out, "", nil

	case NodeSwitch:
		var c SwitchConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		val := Interpolate(c.Value, data)
		for _, cs := range c.Cases {
			if val == cs.Equals {
				return val, cs.Port, nil
			}
		}
		return val, "default", nil

	case NodeMerge:
		// Collect the outputs that arrived on incoming edges, in edge order.
		var inputs []any
		for _, ed := range w.Edges {
			if ed.To != n.ID {
				continue
			}
			if up, ok := data[ed.From].(map[string]any); ok {
				inputs = append(inputs, up["output"])
			}
		}
		return inputs, "", nil

	case NodeApproval:
		var c ApprovalConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		capability := strings.TrimSpace(c.Capability)
		if capability == "" {
			capability = "workflow.approve"
		}
		desc := Interpolate(c.Description, data)
		out := e.approve(ctx, approval.SubmitSpec{
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

	case NodeSubflow:
		var c SubflowConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		depth, _ := ctx.Value(wfDepthKey{}).(int)
		if depth+1 >= maxSubflowDepth {
			return nil, "", fmt.Errorf("subworkflow nesting deeper than %d refused", maxSubflowDepth)
		}
		var subPayload any
		if strings.TrimSpace(c.Payload) != "" {
			subPayload = parseMaybeJSONValue(Interpolate(c.Payload, data))
		}
		subCtx := context.WithValue(ctx, wfDepthKey{}, depth+1)
		subRes, err := e.Run(subCtx, corr, c.Workflow, subPayload, prov)
		if err != nil {
			return nil, "", fmt.Errorf("subworkflow %s: %w", c.Workflow, err)
		}
		return map[string]any{"executed": subRes.Executed, "outputs": subRes.Outputs}, "", nil

	case NodePipeline:
		var c PipelineConfig
		if err := json.Unmarshal(n.Config, &c); err != nil {
			return nil, "", err
		}
		return e.execPipelineNode(ctx, corr, n.ID, c, data)

	default:
		return nil, "", fmt.Errorf("unknown node type %q", n.Type)
	}
}

func (e *Engine) execPipelineNode(ctx context.Context, corr, nodeID string, c PipelineConfig, data map[string]any) (any, string, error) {
	steps := map[string]any{}
	var last any
	for _, step := range c.Steps {
		stepData := copyWorkflowData(data)
		stepData["steps"] = steps
		args := strings.TrimSpace(Interpolate(string(step.Args), stepData))
		if args == "" {
			args = "{}"
		}
		out, _, err := e.invokeWorkflowTool(ctx, corr, step.Tool, "wf-"+nodeID+"-"+step.ID, json.RawMessage(args))
		if err != nil {
			return nil, "", fmt.Errorf("pipeline step %s: %w", step.ID, err)
		}
		if len(strings.TrimSpace(string(step.OutputSchema))) > 0 {
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, "", fmt.Errorf("pipeline step %s output cannot be encoded as JSON: %w", step.ID, err)
			}
			def := toolapi.ToolDef{Name: "pipeline." + nodeID + "." + step.ID + ".output", InputSchema: step.OutputSchema}
			if err := schema.ValidateToolInput(def, raw); err != nil {
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
func (e *Engine) invokeWorkflowTool(ctx context.Context, corr, toolName, callID string, args json.RawMessage) (any, string, error) {
	if e.ports.InvokeTool == nil {
		return nil, "", fmt.Errorf("workflow: no tool invoker available")
	}
	// Node labels repeat across runs, nested graphs and retry attempts. Keep
	// each physical invocation distinct for policy/result and latency joins.
	callID += "-" + ulid.New()
	out, err := e.ports.InvokeTool(ctx, corr, callID, toolName, args)
	if err != nil {
		return nil, "", err
	}
	if out.IsError {
		return nil, "", fmt.Errorf("tool %s failed: %s", toolName, truncateForErr(out.Output))
	}
	return parseMaybeJSON(out.Output), "", nil
}

// The port call sites, each one honest about a missing capability.
func (e *Engine) complete(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if e.ports.Complete == nil {
		return nil, errors.New("workflow: no provider configured")
	}
	return e.ports.Complete(ctx, corr, taskType, req)
}

func (e *Engine) runCode(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, error) {
	if e.ports.RunCode == nil {
		return toolapi.Result{}, errors.New("workflow: no code sandbox available")
	}
	return e.ports.RunCode(ctx, corr, callPrefix, language, code, input)
}

func (e *Engine) scriptRunnerReady() bool {
	return e.ports.ScriptRunnerReady != nil && e.ports.ScriptRunnerReady()
}

func (e *Engine) approve(ctx context.Context, spec approval.SubmitSpec) approval.Outcome {
	if e.ports.Approve == nil {
		return approval.Outcome{Decision: approval.DecisionDeny, Reason: "workflow: no approval registry available"}
	}
	return e.ports.Approve(ctx, spec)
}

func (e *Engine) model(ctx context.Context) string {
	if e.ports.Model == nil {
		return ""
	}
	return e.ports.Model(ctx)
}
