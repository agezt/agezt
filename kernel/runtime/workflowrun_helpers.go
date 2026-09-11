// SPDX-License-Identifier: MIT

// Workflow execution helpers: invokeWorkflowTool + workflowItems + withItem + parseMaybeJSONValue + evalCondition + parseMaybeJSON + truncateForErr.
// Code extracted from workflowrun.go during the Day-47 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/workflow"
)


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
