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
	childCorr := k.NewCorrelation()
	actor := "subagent-" + childCorr

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
		Actor:         actor,
		CorrelationID: childCorr,
		Policy:        k.policyHook,
	}, task)
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", childCorr, err)
	}
	return answer, nil
}
