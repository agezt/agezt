// SPDX-License-Identifier: MIT

package runtime

// Workflow copilot (M802): turn a plain-language description into a
// validated workflow graph. The provider sees the full node-type reference
// and must answer with ONE JSON object in the engine's own schema; the
// kernel extracts it, auto-lays-out the canvas positions, and runs the same
// Validate the save path uses. A draft that fails to parse or validate gets
// exactly one repair round-trip (the error goes back to the model verbatim).
// The result is returned UNSAVED — the canvas (or the caller) reviews and
// saves explicitly, so the copilot can never silently install automation.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/workflow"

	"github.com/agezt/agezt/internal/apperrors"
)

// workflowDraftSystem is the copilot's contract: the node library, the
// template syntax, and the strict-JSON output rule. Kept in lockstep with
// kernel/workflow validation — if a rule changes there, change it here.
const workflowDraftSystem = `You design workflows for the agezt workflow engine. Answer with EXACTLY ONE JSON object — no prose, no markdown fences.

Schema:
{"name":"<lowercase, [a-z0-9][a-z0-9._-]{0,63}>","description":"<one line>","nodes":[{"id":"<[a-z0-9][a-z0-9_-]*>","type":"<type>","label":"<short human label>","config":{...}}],"edges":[{"from":"<id>","to":"<id>","port":"<optional>"}]}

Node types and their config:
- trigger (EXACTLY ONE, no incoming edges): {"kind":"manual"} | {"kind":"cron","interval_sec":<int >=30>} | {"kind":"cron","daily_at":"HH:MM"} | {"kind":"event","subject":"<glob, e.g. task.failed or memory.> — workflow.* is forbidden>"} | {"kind":"webhook","secret":"<random string, >=12 chars>","reply":<optional bool — true makes the POST synchronous and returns the run's outputs to the caller>} (external systems then POST /hooks/<workflow-name> with header X-Agezt-Secret; the request body arrives as {{trigger.payload.body}})
- tool: {"tool":"<tool name>","args":{...}} — one governed tool call; args values may use templates.
- llm: {"prompt":"...","system":"<optional>","model":"<optional, blank = default>"} — one completion.
- condition: {"left":"...","op":"equals|not_equals|contains|not_empty|empty|gt|lt","right":"..."} — outgoing edges MUST use port "true" or "false".
- transform: {"template":"..."} — pure template, output = rendered string.
- delay: {"seconds":<int 1..600>}
- http: {"method":"GET|POST","url":"...","headers":{...},"body":"<optional>"}
- code: {"language":"python|node|deno","code":"<script: reads its JSON input from ./stdin.txt, prints the result to stdout>","input":"<optional templated JSON>"}
- map: {"items":"{{<node>.output.<path>}}","template":"<per-item, use {{item}}, {{item.field}}, {{index}}>"}
- filter: {"items":"{{...}}","left":"{{item.field}}","op":"...","right":"..."} — keeps matching items.
- switch: {"value":"{{...}}","cases":[{"equals":"x","port":"<port>"}]} — outgoing edges use the declared case ports or "default".
- merge: {"mode":"any|all"} — joins branches; "all" waits for every incoming edge.
- approval: {"description":"<what the human operator reads>"} — blocks until granted.
- subworkflow: {"workflow":"<stored name>","payload":"<optional templated JSON>"}

Data flow: {{trigger.payload}} is the start payload; {{<node_id>.output}} is an upstream node's output ({{<node_id>.output.<dotted.path>}} digs into JSON). Nodes that can fail (tool/llm/http/code/approval/subworkflow) may wire an edge with port "error" — then a failure runs the error branch with {{<node_id>.output.error}} instead of failing the run.

Reliability (per node, OUTSIDE config, next to id/type/label): "timeout_sec" (1..600, any non-trigger node) bounds one attempt; "retries" (0..5) and "retry_delay_sec" (0..60) re-run a FAILABLE node on failure — use them on http/tool/code nodes that talk to flaky things.

Rules: ids are short snake_case; give every non-trigger node a label; the graph must be acyclic and every node reachable from the trigger; prefer the http node for web requests and the code node for computation; do NOT invent node types or config keys. Omit x/y (the canvas lays out automatically).`

// errDraftEmpty is returned when the description is blank.
var errDraftEmpty = errors.New("workflow draft: a description is required")

// DraftWorkflow asks the configured provider to design a workflow from a
// plain-language description. name, when non-empty, overrides whatever the
// model chose (the canvas knows the workflow it's editing). The returned
// workflow is validated and auto-laid-out but NOT saved.
func (k *Kernel) DraftWorkflow(ctx context.Context, corr, name, description string) (workflow.Workflow, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return workflow.Workflow{}, errDraftEmpty
	}
	basePrompt := "Design a workflow for this request:\n\n" + description
	return k.draftLoop(ctx, corr, basePrompt, name, "draft")
}

// RefineWorkflow (M805) revises an existing graph from a plain-language
// instruction: the provider sees the CURRENT graph JSON (the canvas's truth,
// unsaved edits included) plus the change request, and answers with the full
// revised graph under the same contract. The base's name is preserved and
// the result is returned UNSAVED, exactly like a fresh draft.
func (k *Kernel) RefineWorkflow(ctx context.Context, corr string, base workflow.Workflow, instruction string) (workflow.Workflow, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return workflow.Workflow{}, errors.New("workflow refine: an instruction is required")
	}
	if err := workflow.Validate(base); err != nil {
		return workflow.Workflow{}, apperrors.WrapSimple("workflow refine: base graph", err)
	}
	baseJSON, err := json.Marshal(map[string]any{
		"name": base.Name, "description": base.Description,
		"nodes": base.Nodes, "edges": base.Edges,
	})
	if err != nil {
		return workflow.Workflow{}, apperrors.WrapSimple("workflow refine", err)
	}
	basePrompt := "Here is an existing workflow:\n\n" + string(baseJSON) +
		"\n\nRevise it per this request (return the COMPLETE revised workflow, keeping unrelated nodes, ids, and positions unchanged):\n\n" + instruction
	return k.draftLoop(ctx, corr, basePrompt, base.Name, "refine")
}

// draftLoop runs the shared design conversation: up to two provider
// round-trips (the attempt and one repair carrying the exact rejection),
// journaling workflow.drafted on success. mode tags the journal payload so
// `agt why` distinguishes fresh drafts from refinements.
func (k *Kernel) draftLoop(ctx context.Context, corr, basePrompt, name, mode string) (workflow.Workflow, error) {
	if k.cfg.Provider == nil {
		return workflow.Workflow{}, errors.New("workflow " + mode + ": no provider configured")
	}
	prompt := basePrompt
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
			Model:    k.cfg.Model,
			System:   workflowDraftSystem,
			TaskType: "workflow", // same routing/metering class as llm nodes
			Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}},
		})
		if err != nil {
			return workflow.Workflow{}, apperrors.WrapSimplef("workflow %s", err, mode)
		}
		w, err := parseWorkflowDraft(resp.Message.Content, name)
		if err == nil {
			_, _ = k.bus.Publish(event.Spec{
				Subject: "workflow." + w.Name, Kind: event.KindWorkflowDrafted, Actor: "workflow",
				CorrelationID: corr,
				Payload: map[string]any{
					"name": w.Name, "nodes": len(w.Nodes), "edges": len(w.Edges),
					"attempt": attempt, "mode": mode,
				},
			})
			return w, nil
		}
		lastErr = err
		// One repair round-trip: the model sees its own output and the exact
		// validation error, and tries again.
		prompt = basePrompt +
			"\n\nYour previous answer was rejected: " + err.Error() +
			"\n\nPrevious answer:\n" + resp.Message.Content +
			"\n\nReturn a corrected JSON object."
	}
	return workflow.Workflow{}, apperrors.WrapSimplef("workflow %s", lastErr, mode)
}

// parseWorkflowDraft extracts the JSON object from a model answer, decodes
// it as a workflow, applies the name override + auto-layout, and validates.
func parseWorkflowDraft(answer, nameOverride string) (workflow.Workflow, error) {
	raw, err := extractJSONObject(answer)
	if err != nil {
		return workflow.Workflow{}, err
	}
	var w workflow.Workflow
	dec := json.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&w); err != nil {
		return workflow.Workflow{}, apperrors.WrapSimple("decode workflow JSON", err)
	}
	if strings.TrimSpace(nameOverride) != "" {
		w.Name = strings.TrimSpace(nameOverride)
	}
	// A draft is a proposal: id/timestamps/enabled are the store's business.
	w.ID, w.CreatedMS, w.UpdatedMS, w.Enabled = "", 0, 0, false
	autoLayoutWorkflow(&w)
	if err := workflow.Validate(w); err != nil {
		return workflow.Workflow{}, err
	}
	return w, nil
}

// extractJSONObject returns the first balanced {...} in s, tolerating prose
// and markdown fences around it (string-literal aware).
func extractJSONObject(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", errors.New("no JSON object in the answer")
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("unbalanced JSON object in the answer")
}

// autoLayoutWorkflow assigns canvas positions when the draft has none:
// BFS depth from the trigger becomes the row, order within a row the column.
// Presentation only — never semantics — so a partial layout is left alone.
func autoLayoutWorkflow(w *workflow.Workflow) {
	for _, n := range w.Nodes {
		if n.X != 0 || n.Y != 0 {
			return // the model (or a human) already placed things
		}
	}
	depth := map[string]int{}
	adj := map[string][]string{}
	for _, e := range w.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	var queue []string
	for _, n := range w.Nodes {
		if n.Type == workflow.NodeTrigger {
			depth[n.ID] = 0
			queue = append(queue, n.ID)
		}
	}
	// Layout runs BEFORE Validate, so a cyclic draft must not spin this BFS
	// forever — bound the relaxations; Validate rejects the cycle right after.
	steps := (len(w.Nodes) + 1) * (len(w.Edges) + 1)
	head := 0
	for head < len(queue) && steps > 0 {
		steps--
		id := queue[head]
		head++
		for _, next := range adj[id] {
			if d, seen := depth[next]; !seen || depth[id]+1 > d {
				depth[next] = depth[id] + 1
				queue = append(queue, next)
			}
		}
	}
	col := map[int]int{}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		d := depth[n.ID] // unreachable nodes land on row 0 next to the trigger
		n.X = float64(40 + col[d]*240)
		n.Y = float64(40 + d*150)
		col[d]++
	}
}
