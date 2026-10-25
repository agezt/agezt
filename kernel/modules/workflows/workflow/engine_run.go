// SPDX-License-Identifier: MIT

package workflow

// The run: one stored graph, executed node by node, journaled as it goes.
//
// Provenance (why the run exists) travels with the call, not with a context
// key the module cannot read: the host fills Provenance in from its own wake
// context and the module stamps it on every workflow.* event it publishes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// ErrHalted is returned when the host reports that the kernel is halted. The
// host translates it into its own sentinel, so the caller sees one error.
//
// The sentinel lives in the contract layer (kernel/contract/workflowapi) — the
// HOST reports it, and the host may not name a module's error. The name stays
// here as an alias.

// workflowStepCap is a defense-in-depth bound on executed steps per run —
// validation already rejects cycles, so this can only fire on an engine bug.
const workflowStepCap = 256

// Run executes one stored workflow under corr. payload becomes
// {{trigger.payload}}. Halted kernels refuse; the run respects ctx
// cancellation between and inside nodes. The first failing node fails the
// run (error branching arrives with M800).
func (e *Engine) Run(ctx context.Context, corr, ref string, payload any, prov Provenance) (RunResult, error) {
	if e.halted() {
		return RunResult{}, ErrHalted
	}
	w, found := e.store.Get(ref)
	if !found {
		return RunResult{}, ErrNotFound
	}
	if err := Validate(w); err != nil { // defense: stores can predate rules
		return RunResult{}, err
	}
	ctx = toolapi.WithCorrelation(ctx, corr)
	runMeta := provenanceMap(ctx, prov)

	e.publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowStarted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeRunPayload(map[string]any{"id": w.ID, "name": w.Name, "nodes": len(w.Nodes)}, runMeta),
	})

	res, err := e.runWorkflowGraph(ctx, corr, w, payload, prov)
	if err != nil {
		e.publish(event.Spec{
			Subject: "workflow." + w.Name, Kind: event.KindWorkflowFailed, Actor: "workflow",
			CorrelationID: corr,
			Payload:       mergeRunPayload(map[string]any{"id": w.ID, "name": w.Name, "error": err.Error(), "executed": res.Executed}, runMeta),
		})
		return res, err
	}
	e.publish(event.Spec{
		Subject: "workflow." + w.Name, Kind: event.KindWorkflowCompleted, Actor: "workflow",
		CorrelationID: corr,
		Payload:       mergeRunPayload(map[string]any{"id": w.ID, "name": w.Name, "executed": res.Executed}, runMeta),
	})
	return res, nil
}

// provenanceMap is the run's audit stamp: what fired it and who ran it.
func provenanceMap(ctx context.Context, prov Provenance) map[string]any {
	source := strings.TrimSpace(prov.Source)
	if source == "" {
		source = "manual"
	}
	agentSlug := strings.TrimSpace(toolapi.AgentFromContext(ctx))
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
	if prov.ScheduleID != "" {
		out["schedule_id"] = prov.ScheduleID
	}
	if prov.StandingID != "" {
		out["standing_id"] = prov.StandingID
	}
	if prov.StandingName != "" {
		out["standing_name"] = prov.StandingName
	}
	if prov.TriggerSubject != "" {
		out["trigger_subject"] = prov.TriggerSubject
	}
	if prov.ParentCorrelation != "" {
		out["parent_correlation_id"] = prov.ParentCorrelation
	}
	return out
}

func mergeRunPayload(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func (e *Engine) halted() bool {
	return e.ports.Halted != nil && e.ports.Halted()
}

// runWorkflowGraph walks the graph from the trigger, executing each node
// once and threading its output into the templates of the nodes downstream.
func (e *Engine) runWorkflowGraph(ctx context.Context, corr string, w Workflow, payload any, prov Provenance) (RunResult, error) {
	// Outgoing edges grouped by (from, port); incoming counts for merge "all".
	edges := map[string]map[string][]string{}
	indegree := map[string]int{}
	for _, ed := range w.Edges {
		if edges[ed.From] == nil {
			edges[ed.From] = map[string][]string{}
		}
		edges[ed.From][ed.Port] = append(edges[ed.From][ed.Port], ed.To)
		indegree[ed.To]++
	}

	data := map[string]any{"trigger": map[string]any{"payload": payload}}
	res := RunResult{Outputs: map[string]any{}}
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
		if node.Type == NodeMerge && mergeMode(node) == "all" && tokens[id] < indegree[id] {
			continue
		}
		executed[id] = true

		inputPreview := nodeInputPreview(node, data)
		output, port, attempts, err := e.execNodeWithReliability(ctx, corr, node, w, data, payload, prov)
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
		e.publish(event.Spec{
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

func mergeMode(n *Node) string {
	var c MergeConfig
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
func (e *Engine) execNodeWithReliability(ctx context.Context, corr string, n *Node, w Workflow, data map[string]any, payload any, prov Provenance) (any, string, int, error) {
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
		output, port, err = e.execWorkflowNode(actx, corr, n, w, data, payload, prov)
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
func nodeInputPreview(n *Node, data map[string]any) string {
	switch n.Type {
	case NodeTool:
		var c ToolConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(Interpolate(string(c.Args), data))
	case NodeLLM:
		var c LLMConfig
		_ = json.Unmarshal(n.Config, &c)
		return Interpolate(c.Prompt, data)
	case NodeCondition:
		var c ConditionConfig
		_ = json.Unmarshal(n.Config, &c)
		return Interpolate(c.Left, data) + " " + c.Op + " " + Interpolate(c.Right, data)
	case NodeHTTP:
		var c HTTPConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.ToUpper(strings.TrimSpace(c.Method)) + " " + Interpolate(c.URL, data)
	case NodeCode:
		var c CodeConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(Interpolate(c.Input, data))
	case NodeMap, NodeFilter:
		var c struct {
			Items string `json:"items"`
		}
		_ = json.Unmarshal(n.Config, &c)
		return Interpolate(c.Items, data)
	case NodeSwitch:
		var c SwitchConfig
		_ = json.Unmarshal(n.Config, &c)
		return Interpolate(c.Value, data)
	case NodeSubflow:
		var c SubflowConfig
		_ = json.Unmarshal(n.Config, &c)
		return strings.TrimSpace(Interpolate(c.Payload, data))
	case NodePipeline:
		var c PipelineConfig
		_ = json.Unmarshal(n.Config, &c)
		parts := make([]string, 0, len(c.Steps))
		for _, step := range c.Steps {
			args := strings.TrimSpace(Interpolate(string(step.Args), data))
			if args == "" {
				args = "{}"
			}
			parts = append(parts, strings.TrimSpace(step.Tool)+"("+args+")")
		}
		return strings.Join(parts, " -> ")
	}
	return ""
}
