// SPDX-License-Identifier: MIT

package planner

// Operator-driven plan refinement (M1.uu). Re-runs the planner
// against an existing plan + a free-text feedback message; returns
// a revised plan that still satisfies the same structural rules
// the original plan did (same validators in parseAndValidate).
//
// **Why not auto-replan.** The package-level doc explicitly rules
// out mid-execution recursion: "they invite runaway behaviour the
// audit story can't keep up with." Refinement is the safe middle
// ground — the operator looks at a failed/incomplete run, writes
// a sentence describing what to change, and the planner produces
// a new candidate. The operator decides whether to execute it.
// Every refinement loop has a human pause; no LLM-to-LLM cascade.
//
// **What gets passed back to the LLM.** The original plan JSON
// AND the operator feedback, both inside the same prompt. The
// model is asked to produce a *complete* replacement plan, not a
// diff — diff-style edits would force a separate merge layer and
// any merge bug would silently corrupt the plan. Whole-replacement
// + re-validation is the simplest sound approach.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// RefineSystemPrompt is the system message for refinement calls.
// Exported (like SystemPrompt) so operators can override it.
var RefineSystemPrompt = `You are refining an existing execution plan based on operator feedback.

You will receive:
  1. The CURRENT plan as JSON.
  2. OPERATOR FEEDBACK describing what to change.

Your job: produce a COMPLETE REPLACEMENT plan that incorporates the feedback.

OUTPUT FORMAT — return EXACTLY one JSON object inside a fenced code block, same shape as the planner's initial format:

` + "```json" + `
{
  "name": "<short label>",
  "max_parallel": <int 1-8>,
  "nodes": [
    {"id": "<unique-snake-case>", "kind": "loop", "intent": "<imperative instruction>", "deps": ["<id>", ...]},
    {"id": "<unique-snake-case>", "kind": "gate", "capability": "<dotted.label>", "description": "<human prompt>", "deps": ["<id>"]}
  ]
}
` + "```" + `

RULES (identical to initial planning):
1. 1-6 nodes total. Prefer fewer, larger nodes.
2. "deps" lists node ids that must finish before this one starts. Empty array for root nodes.
3. No cycles. Every id in "deps" must exist as another node's id.
4. Node kinds: "loop" (agent tool-loop) or "gate" (human approval).
5. snake_case ids; verb-noun shape.
6. Do NOT include commentary, explanation, or diff markers outside the fenced code block. Return the WHOLE new plan, not just the changes.

REFINEMENT PRINCIPLES:
- Preserve node ids from the original where the feedback doesn't require changing them — this helps the operator see what's the same.
- When the feedback says "remove node X", actually drop it AND remove dangling deps that pointed at it.
- When the feedback says "add a gate before X", add a new gate node and rewrite X's deps.
- When the feedback contradicts the original intent, follow the feedback — it's the latest signal.
`

// Refine produces a new plan derived from `original` plus the
// operator's `feedback`. Returns the raw replacement JSON plus the
// validated Plan, mirroring Generate's signature so the CLI plumbing
// can share code.
func Refine(ctx context.Context, cfg Config, original Plan, feedback string) (rawJSON string, plan Plan, err error) {
	if cfg.Provider == nil {
		return "", Plan{}, errors.New("planner refine: Provider required")
	}
	if len(original.Nodes) == 0 {
		return "", Plan{}, errors.New("planner refine: original plan has no nodes")
	}
	if strings.TrimSpace(feedback) == "" {
		return "", Plan{}, errors.New("planner refine: feedback required (use Generate for unconstrained planning)")
	}

	origJSON, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		return "", Plan{}, fmt.Errorf("planner refine: marshal original: %w", err)
	}

	maxTok := cfg.MaxTokens
	if maxTok <= 0 {
		maxTok = DefaultMaxTokens
	}
	sys := cfg.SystemOverride
	if strings.TrimSpace(sys) == "" {
		sys = RefineSystemPrompt
	}

	user := "CURRENT PLAN:\n```json\n" + string(origJSON) + "\n```\n\nOPERATOR FEEDBACK:\n" + feedback
	req := agent.CompletionRequest{
		System:    sys,
		Model:     cfg.Model,
		MaxTokens: maxTok,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: user},
		},
		TaskType: TaskType, // same routing/pricing bucket as initial planning
	}
	resp, err := cfg.Provider.Complete(ctx, req)
	if err != nil {
		return "", Plan{}, fmt.Errorf("planner refine: LLM call: %w", err)
	}
	body := resp.Message.Content
	if strings.TrimSpace(body) == "" {
		return "", Plan{}, errors.New("planner refine: empty response from LLM")
	}

	rawJSON, err = extractJSONBlock(body)
	if err != nil {
		return "", Plan{}, fmt.Errorf("planner refine: %w (response was: %s)", err, snippet(body))
	}
	plan, err = parseAndValidate(rawJSON)
	if err != nil {
		return rawJSON, Plan{}, fmt.Errorf("planner refine: %w", err)
	}
	return rawJSON, plan, nil
}
