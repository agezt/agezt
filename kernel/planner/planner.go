// SPDX-License-Identifier: MIT

// Package planner generates `scheduler.Plan`-shaped JSON from a
// natural-language intent by asking the configured Provider to
// emit a DAG. The output is the same JSON shape `agt plan <file>`
// already executes (handlePlan in kernel/controlplane), so
// running a generated plan is identical to running a hand-authored
// one — same node types, same scheduler, same audit trail.
//
// **Scope (M1.v).** Two node kinds: `loop` and `gate`. The
// scheduler supports both; the planner can emit either. Future
// node kinds (llm/tool/agent — SPEC-02 §4.2) will need both a
// scheduler implementation and a planner-prompt update.
//
// **No agentic-meta nonsense.** The planner is a *single*
// LLM call that returns a static DAG. It does NOT recurse into
// sub-planners, it does NOT re-plan mid-execution, and it does
// NOT call tools during planning. Those are real capabilities,
// but they're deliberately out of scope for v1 — they invite
// runaway behaviour the audit story can't keep up with.
//
// **Output validation.** We don't trust the model. After parsing
// the JSON the planner verifies:
//
//   - at least one node
//   - every node id is unique and non-empty
//   - every `deps` reference resolves
//   - no cycles (handled by the scheduler's existing topological
//     sort, but we duplicate the check here so the operator sees
//     "your planner emitted a bad DAG" rather than "scheduler
//     refused")
//   - every node kind ∈ {loop, gate}
//
// Failures return a descriptive error; the prompt asks for JSON
// in a fenced code block, and we strip the fence before parsing.
package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	intentmodel "github.com/agezt/agezt/kernel/intent"
)

// TaskType is the per-task-type routing hint (M1.cc) the planner
// stamps onto every CompletionRequest. Operators wire this through
// AGEZT_TASK_ROUTES="plan=anthropic" (etc.) to pin planning calls
// to a specific provider without affecting in-loop calls.
const TaskType = "plan"

// SystemPrompt is the instruction we send to the LLM. Kept as an
// exported var so operators can override it (e.g. to nudge the
// planner toward more gates, or to inject domain-specific rules).
var SystemPrompt = `You are a planner. Convert the supplied INTENT_FRAME into a JSON execution plan.

You do not receive the user's raw utterance. Treat the INTENT_FRAME as the only
authority for what the user meant. If the frame is underdetermined or contains a
harmful_reading, preserve that uncertainty with a targeted "gate" node before
any irreversible or high-blast-radius action.

OUTPUT FORMAT — return EXACTLY one JSON object inside a fenced code block:

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

NODE KINDS:
- "loop": runs one agent tool-loop with the given intent. The intent should be a complete imperative instruction.
- "gate": pauses for human approval. Use SPARINGLY — only when an irreversible or high-blast-radius action is about to happen.

RULES:
1. Decompose the intent into 1-6 nodes. Prefer fewer, larger nodes.
2. "deps" lists node ids that must finish before this one starts. Empty array for root nodes.
3. No cycles. Every id in "deps" must exist as another node's id.
4. If the intent is a single self-contained task, return a 1-node plan.
5. Use snake_case for node ids. Use a verb-noun shape: "research_topic", "draft_summary".
6. If ambiguity_score >= 0.6 or underdetermined=true, add a gate before mutation/destructive execution.
7. Gate descriptions must name the ambiguous scope and the plausible harmful interpretation.
8. Do NOT include commentary, explanation, or text outside the fenced code block.

`

// Config tunes a generation call.
type Config struct {
	// Provider is the LLM provider used to generate the plan.
	// Typically the same provider the operator's runs use, but the
	// caller can pass a cheaper/faster one (planners benefit less
	// from frontier models than executors do).
	Provider agent.Provider
	// Model overrides the provider's default model when set.
	// Empty falls back to the provider's own default.
	Model string
	// MaxTokens caps the planner's response. 2048 is generous —
	// even a 6-node plan with full intents fits in ~600 tokens.
	MaxTokens int
	// SystemOverride replaces the package-level SystemPrompt for
	// this call only. Empty uses the default.
	SystemOverride string
}

// DefaultMaxTokens is the conservative cap for planner responses.
const DefaultMaxTokens = 2048

// Generate asks the provider for a plan satisfying the intent and
// returns the validated plan JSON as a string (the same shape
// `agt plan <file.json>` already accepts).
//
// Returns the raw JSON string, the parsed Plan, and any error from
// either the LLM call or post-hoc validation. Returning both the
// raw and parsed forms saves the caller a re-marshal when piping
// the JSON to the scheduler's handlePlan over the wire.
func Generate(ctx context.Context, cfg Config, intent string) (rawJSON string, plan Plan, err error) {
	if strings.TrimSpace(intent) == "" {
		return "", Plan{}, errors.New("planner: intent required")
	}
	return GenerateFromIntent(ctx, cfg, intentmodel.Interpret(intent))
}

// GenerateFromIntent asks the provider for a plan from a formal intent frame,
// not the raw user utterance. This is the planner-side interpreter/executor
// boundary: the LLM planner receives explicit ambiguity and harmful-reading
// metadata instead of silently collapsing the user's words into one reading.
func GenerateFromIntent(ctx context.Context, cfg Config, frame intentmodel.Frame) (rawJSON string, plan Plan, err error) {
	if cfg.Provider == nil {
		return "", Plan{}, errors.New("planner: Provider required")
	}
	if strings.TrimSpace(frame.CanonicalIntent) == "" {
		return "", Plan{}, errors.New("planner: intent frame required")
	}
	maxTok := cfg.MaxTokens
	if maxTok <= 0 {
		maxTok = DefaultMaxTokens
	}
	sys := cfg.SystemOverride
	if strings.TrimSpace(sys) == "" {
		sys = SystemPrompt
	}
	intentFrame, err := marshalIntentFrame(frame)
	if err != nil {
		return "", Plan{}, err
	}

	req := agent.CompletionRequest{
		System:    sys,
		Model:     cfg.Model,
		MaxTokens: maxTok,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: intentFrame},
		},
		// Hint the Governor's per-task-type routing (M1.cc): operators
		// can pin planner LLM calls to a specific provider via
		// AGEZT_TASK_ROUTES="plan=anthropic" (etc.) without affecting
		// the rest of the agent loop's calls. Default routing applies
		// when the env var is unset.
		TaskType: TaskType,
		// Request structured output (M313) — plan generation is SPEC-10's
		// canonical "reliability over free-form parsing" case: the response is
		// parsed as JSON below. Providers with a native JSON mode (OpenAI &
		// compatibles, Gemini, Ollama) constrain decoding to valid JSON; those
		// without one (Anthropic) ignore the flag and the prompt's explicit
		// JSON instruction still applies. Safe to set unconditionally — the
		// prompt already names JSON (satisfying OpenAI's json_object precondition).
		JSONMode: true,
	}
	resp, err := cfg.Provider.Complete(ctx, req)
	if err != nil {
		return "", Plan{}, fmt.Errorf("planner: LLM call: %w", err)
	}
	body := resp.Message.Content
	if strings.TrimSpace(body) == "" {
		return "", Plan{}, errors.New("planner: empty response from LLM")
	}

	rawJSON, err = extractJSONBlock(body)
	if err != nil {
		return "", Plan{}, fmt.Errorf("planner: %w (response was: %s)", err, snippet(body))
	}

	plan, err = parseAndValidate(rawJSON)
	if err != nil {
		return rawJSON, Plan{}, fmt.Errorf("planner: %w", err)
	}
	if err := validateIntentBoundary(frame, plan); err != nil {
		return rawJSON, Plan{}, fmt.Errorf("planner: %w", err)
	}
	plan.Intent = &frame
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return rawJSON, Plan{}, fmt.Errorf("planner: marshal plan with intent metadata: %w", err)
	}
	rawJSON = string(raw)
	return rawJSON, plan, nil
}

func marshalIntentFrame(frame intentmodel.Frame) (string, error) {
	raw, err := json.MarshalIndent(frame, "", "  ")
	if err != nil {
		return "", fmt.Errorf("planner: marshal intent frame: %w", err)
	}
	return "INTENT_FRAME:\n" + string(raw), nil
}

// Plan mirrors the JSON shape the scheduler's control-plane handler
// (kernel/controlplane.planSpec / planNodeSpec) accepts. Duplicated
// here as a public type so callers (CLI, library users) have a
// concrete type to inspect without importing controlplane internals.
type Plan struct {
	Name        string             `json:"name,omitempty"`
	MaxParallel int                `json:"max_parallel,omitempty"`
	Intent      *intentmodel.Frame `json:"intent,omitempty"`
	Nodes       []Node             `json:"nodes"`
}

// Node is one planner-emitted node. Fields are a union over
// loop/gate; unset ones for a given kind are omitted via omitempty.
type Node struct {
	ID   string   `json:"id"`
	Kind string   `json:"kind"`
	Deps []string `json:"deps,omitempty"`
	// loop:
	Intent string `json:"intent,omitempty"`
	// gate:
	Capability  string `json:"capability,omitempty"`
	Description string `json:"description,omitempty"`
}

// ----- JSON extraction + validation -----

// extractJSONBlock pulls the first fenced JSON block out of the
// model's reply. Accepts both ```json ... ``` and bare ``` ... ```
// fences (some models drop the language tag). If no fence is
// present, the entire response is treated as JSON — Some models
// (especially those trained for tool-use) skip fences when asked
// for "just JSON."
func extractJSONBlock(body string) (string, error) {
	body = strings.TrimSpace(body)
	// Strip fences if present.
	if strings.HasPrefix(body, "```") {
		// Skip the opening line entirely (e.g. ```json\n).
		_, inner, ok := strings.Cut(body, "\n")
		if !ok {
			return "", errors.New("malformed fenced block (no newline after opening fence)")
		}
		closeIdx := strings.LastIndex(inner, "```")
		if closeIdx < 0 {
			return "", errors.New("malformed fenced block (no closing fence)")
		}
		return strings.TrimSpace(inner[:closeIdx]), nil
	}
	// No fence — assume the whole body is JSON.
	if !strings.HasPrefix(body, "{") {
		return "", errors.New("response is not JSON and not fenced")
	}
	return body, nil
}

// ValidateJSON is the operator-facing entry point for plan
// validation. Decodes the JSON, runs every structural check the
// planner applies to LLM-generated plans (no-empty, unique ids,
// kind ∈ {loop, gate}, all deps resolve, DAG is acyclic), and
// returns the typed Plan on success.
//
// Used by `agt plan validate <file>` so operators authoring plans
// by hand can verify them in CI without spinning up the daemon —
// matches the validators the daemon applies before executing a
// plan submitted via `agt plan <file>`.
func ValidateJSON(raw []byte) (Plan, error) { return parseAndValidate(string(raw)) }

// parseAndValidate decodes the JSON, runs the structural checks
// described in the package doc, and returns the typed Plan.
func parseAndValidate(raw string) (Plan, error) {
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Plan{}, fmt.Errorf("plan JSON parse: %w", err)
	}
	if len(p.Nodes) == 0 {
		return Plan{}, errors.New("plan has no nodes")
	}
	if p.MaxParallel < 0 {
		return Plan{}, fmt.Errorf("max_parallel must be >= 0 (got %d)", p.MaxParallel)
	}

	ids := make(map[string]struct{}, len(p.Nodes))
	for i, n := range p.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return Plan{}, fmt.Errorf("node[%d]: id is empty", i)
		}
		if _, dup := ids[n.ID]; dup {
			return Plan{}, fmt.Errorf("node[%d]: duplicate id %q", i, n.ID)
		}
		ids[n.ID] = struct{}{}
		switch n.Kind {
		case "loop":
			if strings.TrimSpace(n.Intent) == "" {
				return Plan{}, fmt.Errorf("node %q (loop): intent is empty", n.ID)
			}
		case "gate":
			if strings.TrimSpace(n.Description) == "" {
				return Plan{}, fmt.Errorf("node %q (gate): description is empty", n.ID)
			}
		default:
			return Plan{}, fmt.Errorf("node %q: unknown kind %q (want loop|gate)", n.ID, n.Kind)
		}
	}
	// Dep resolution + cycle check (Kahn-style topological walk).
	if err := validateDAG(p.Nodes, ids); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// validateDAG ensures every dep id exists and there are no cycles.
// We run our own check rather than relying on the scheduler so a
// bad plan from the LLM produces a clear "node X depends on Y
// which doesn't exist" message at the planner boundary.
func validateDAG(nodes []Node, ids map[string]struct{}) error {
	// Reference check.
	for _, n := range nodes {
		for _, d := range n.Deps {
			if d == n.ID {
				return fmt.Errorf("node %q depends on itself", n.ID)
			}
			if _, ok := ids[d]; !ok {
				return fmt.Errorf("node %q: dep %q does not exist", n.ID, d)
			}
		}
	}
	// Cycle check via Kahn (count incoming, repeatedly drop nodes
	// with zero remaining inputs).
	indeg := map[string]int{}
	deps := map[string][]string{}
	for _, n := range nodes {
		indeg[n.ID] = 0
		deps[n.ID] = nil
	}
	for _, n := range nodes {
		for _, d := range n.Deps {
			indeg[n.ID]++
			deps[d] = append(deps[d], n.ID)
		}
	}
	queue := []string{}
	for id, deg := range indeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, downstream := range deps[id] {
			indeg[downstream]--
			if indeg[downstream] == 0 {
				queue = append(queue, downstream)
			}
		}
	}
	if processed != len(nodes) {
		return fmt.Errorf("plan has a cycle (processed %d of %d nodes via topological walk)", processed, len(nodes))
	}
	return nil
}

func validateIntentBoundary(frame intentmodel.Frame, plan Plan) error {
	if !frame.Underdetermined || frame.AmbiguityScore < 0.6 {
		return nil
	}
	nodes := make(map[string]Node, len(plan.Nodes))
	for _, n := range plan.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range plan.Nodes {
		if n.Kind != "loop" {
			continue
		}
		axes := intentmodel.RegretForAction(intentmodel.Action{
			ToolName:    "planner.loop",
			Capability:  "plan.loop",
			EffectClass: "read_only",
			Input:       n.Intent,
		})
		if !intentmodel.RequiresConfirmation(frame, axes) {
			continue
		}
		if !hasGateDependency(n, nodes, map[string]bool{}) {
			return fmt.Errorf("underdetermined intent requires a gate before high-regret loop node %q", n.ID)
		}
	}
	return nil
}

func hasGateDependency(n Node, nodes map[string]Node, seen map[string]bool) bool {
	for _, depID := range n.Deps {
		if seen[depID] {
			continue
		}
		seen[depID] = true
		dep, ok := nodes[depID]
		if !ok {
			continue
		}
		if dep.Kind == "gate" {
			return true
		}
		if hasGateDependency(dep, nodes, seen) {
			return true
		}
	}
	return false
}

// snippet returns the first ~200 chars of s for error messages.
// Truncates so a verbose LLM doesn't dump a multi-kilobyte
// response into the operator's terminal as part of an error.
func snippet(s string) string {
	return strutil.Ellipsis(strings.TrimSpace(s), 200, "...")
}
