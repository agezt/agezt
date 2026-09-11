// SPDX-License-Identifier: MIT

// Workflow execution orchestration: runWorkflowGraph + mergeMode + execNodeWithReliability + wfSnippet + nodeInputPreview.
// Code extracted from workflowrun.go during the Day-47 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/workflow"
)


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