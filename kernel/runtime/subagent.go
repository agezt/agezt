// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// ctxKeyDepth carries the current sub-agent nesting depth so runSubAgent can
// enforce SubAgentMaxDepth and refuse unbounded recursion. It rides the same
// context the agent loop threads into each tool Invoke.
type ctxKeyDepthT struct{}

var ctxKeyDepth = ctxKeyDepthT{}

func depthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeyDepth).(int); ok {
		return v
	}
	return 0
}

// subAgentSystem frames a delegated run: a focused worker that completes one
// task and reports back concisely. The kernel's own System prompt follows.
const subAgentSystem = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."

// subAgentTool is the in-process `delegate` tool (P6-MULTI-01). Its runner is
// wired to k.runSubAgent after the kernel is constructed (the tool is built
// during Open before *Kernel exists).
type subAgentTool struct {
	run func(ctx context.Context, task string) (string, error)
}

func newSubAgentTool() *subAgentTool { return &subAgentTool{} }

func (t *subAgentTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "delegate",
		Description: "Delegate a focused subtask to a fresh sub-agent that works " +
			"autonomously (its own tool-loop) and returns a concise result. Use this " +
			"to parallelise independent subtasks or isolate a self-contained piece of " +
			"work. Issue multiple delegate calls in one turn to fan out concurrently.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the sub-agent. Include all context it needs; it does not see this conversation."
    }
  },
  "required": ["task"]
}`),
	}
}

func (t *subAgentTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if t.run == nil {
		return agent.Result{Output: "sub-agent runner not wired", IsError: true}, nil
	}
	out, err := t.run(ctx, in.Task)
	if err != nil {
		// Surface as a tool error so the lead agent can adapt, not crash.
		return agent.Result{Output: "delegation failed: " + err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: out}, nil
}

// runSubAgent executes a delegated task as a nested agent.Run under a fresh
// child correlation, bounded by SubAgentMaxDepth. The spawn is journaled under
// the PARENT correlation (carrying the child correlation) so `agt why <parent>`
// shows the delegation; the child's own steps live under the child correlation.
func (k *Kernel) runSubAgent(ctx context.Context, task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", errors.New("task required")
	}
	if k.IsHalted() {
		return "", ErrHalted
	}

	depth := depthFromCtx(ctx)
	maxDepth := k.cfg.SubAgentMaxDepth
	if maxDepth <= 0 {
		maxDepth = 1
	}
	if depth >= maxDepth {
		return "", fmt.Errorf("max sub-agent depth %d reached", maxDepth)
	}

	parentCorr := correlationFromCtx(ctx)

	// Fan-out bound (M46): depth caps how DEEP delegation nests; this caps how
	// WIDE a single run fans out. We tally sub-agents per spawning correlation
	// (the lead run, or a sub-agent that itself delegates) in k.fanout and
	// refuse the Nth+1 call with a tool error the lead adapts to. 0 = unbounded
	// (default). A correlation-less spawn (no run context) can't be attributed,
	// so it's left unbounded. The tally is released when the spawning run ends
	// (RunWith's defer for top-level; this function's defer for a nested
	// spawner's own child correlation below).
	if maxFanout := k.cfg.SubAgentMaxFanout; maxFanout > 0 && parentCorr != "" {
		k.mu.Lock()
		n := k.fanout[parentCorr]
		if n >= maxFanout {
			k.mu.Unlock()
			return "", fmt.Errorf("max sub-agent fan-out %d reached", maxFanout)
		}
		k.fanout[parentCorr] = n + 1
		k.mu.Unlock()
	}

	// Spend cap (M48): once this run's sub-agents have collectively spent past
	// SubAgentMaxSpendMicrocents, refuse further delegations — the cost analogue
	// of the fan-out count cap above. The tally is read from the journal, which
	// is durable by the time each prior child returned (bus.Publish appends
	// before it returns), so the previous delegations' spend is already visible
	// here — no in-memory accounting, race-free. Only scanned when the cap is
	// enabled, so it stays off the default path. 0 = unbounded.
	if cap := k.cfg.SubAgentMaxSpendMicrocents; cap > 0 && parentCorr != "" {
		if spent := k.subAgentSpendMicrocents(parentCorr); spent >= cap {
			return "", fmt.Errorf("max sub-agent spend $%.4f reached", float64(cap)/1e9)
		}
	}

	childCorr := k.NewCorrelation()
	actor := "subagent-" + childCorr

	// This child may itself delegate; release its own fan-out tally when it
	// returns so the map doesn't accumulate across a long-lived kernel.
	defer func() {
		k.mu.Lock()
		delete(k.fanout, childCorr)
		k.mu.Unlock()
	}()

	// Journal the spawn under the parent correlation so `agt why <parent>`
	// reveals the delegation and the child correlation to drill into.
	linkCorr := parentCorr
	if linkCorr == "" {
		linkCorr = childCorr
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "agent." + actor + ".subagent",
		Kind:          event.KindSubAgentSpawned,
		Actor:         actor,
		CorrelationID: linkCorr,
		Payload: map[string]any{
			"task":              task,
			"child_correlation": childCorr,
			"depth":             depth + 1,
			"parent":            parentCorr,
		},
	})

	// Child context: bump depth, retarget actor/correlation so the policy hook
	// and approval audit attribute the sub-agent's actions correctly.
	childCtx := context.WithValue(ctx, ctxKeyDepth, depth+1)
	childCtx = context.WithValue(childCtx, ctxKeyActor, actor)
	childCtx = context.WithValue(childCtx, ctxKeyCorrelation, childCorr)

	system := subAgentSystem
	if k.cfg.System != "" {
		system += "\n\n" + k.cfg.System
	}

	answer, err := agent.Run(childCtx, agent.LoopConfig{
		Provider:      k.cfg.Provider,
		Tools:         k.tools,
		Bus:           k.bus,
		Model:         k.cfg.Model,
		System:        system,
		MaxIter:       k.cfg.MaxIter,
		ToolTimeout:   k.cfg.ToolTimeout,
		Actor:         actor,
		CorrelationID: childCorr,
		Policy:        k.policyHook,
	}, task)
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", childCorr, err)
	}
	return answer, nil
}

// subAgentSpendMicrocents sums the spend (budget.consumed cost_microcents, M47)
// of every run descended from parentCorr — its sub-agents and their sub-agents,
// transitively — excluding parentCorr's own direct spend. It backs the M48
// spend cap: a single forward journal pass builds the parent→children links
// (from subagent.spawned) and the per-run spend (from budget.consumed), then
// totals the spend over parentCorr's transitive descendants. Stateless and
// race-free: every prior delegation's spend is already durably journaled by the
// time the next delegate calls this. Only invoked when the cap is enabled.
func (k *Kernel) subAgentSpendMicrocents(parentCorr string) int64 {
	childrenOf := map[string][]string{}
	spendOf := map[string]int64{}
	_ = k.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindSubAgentSpawned:
			if child, parent := spawnLink(e.Payload); child != "" && parent != "" {
				childrenOf[parent] = append(childrenOf[parent], child)
			}
		case event.KindBudgetConsumed:
			if e.CorrelationID != "" {
				spendOf[e.CorrelationID] += budgetCostMicrocents(e.Payload)
			}
		}
		return nil
	})

	// Sum spend over the transitive descendants of parentCorr (BFS over the
	// links), excluding parentCorr itself — the cap bounds sub-agent spend, not
	// the lead's own. A `seen` set guards against a malformed cyclic link.
	var total int64
	seen := map[string]bool{parentCorr: true}
	queue := append([]string{}, childrenOf[parentCorr]...)
	for len(queue) > 0 {
		corr := queue[0]
		queue = queue[1:]
		if seen[corr] {
			continue
		}
		seen[corr] = true
		total += spendOf[corr]
		queue = append(queue, childrenOf[corr]...)
	}
	return total
}

// spawnLink pulls child + parent correlation ids out of a subagent.spawned
// payload (M48 mirror of the control plane's extractSpawnLink). Returns
// ("","") on parse failure so an unparseable link is simply skipped.
func spawnLink(payload json.RawMessage) (child, parent string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Child, p.Parent
}

// budgetCostMicrocents pulls cost_microcents out of a budget.consumed payload
// (M48). Returns 0 on parse failure so an unparseable spend event contributes
// nothing. JSON numbers decode as float64; the integer microcents have no
// fractional part to lose.
func budgetCostMicrocents(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		Cost float64 `json:"cost_microcents"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int64(p.Cost)
}
