// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
)

// Tool is the in-process `delegate` tool (P6-MULTI-01). Its runners are
// wired to the daemon's delegation engine after the kernel is constructed
// (the tool is built during Open before *Kernel exists).
type Tool struct {
	// Run executes a delegation and blocks until the child answers.
	Run func(ctx context.Context, req Request) (string, error)
	// Spawn starts a delegation and returns the spawn id immediately.
	Spawn func(ctx context.Context, req Request) (string, error)
}

// NewTool builds the `delegate` tool.
func NewTool() *Tool { return &Tool{} }

// Definition describes the `delegate` tool.
func (t *Tool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:       "delegate",
		Capability: toolapi.ToolCapability{Name: string(edict.CapDelegate)},
		Description: "Delegate a focused subtask to a fresh sub-agent that works " +
			"autonomously (its own tool-loop) and returns a concise result. LEAD the work: " +
			"break a big task into parts and delegate each — your sub-agents can delegate " +
			"FURTHER, so you can build a leader/worker tree, not just one flat layer. Issue " +
			"multiple delegate calls in one turn to fan out concurrently, or pass " +
			"async=true to get a spawn_id back immediately and keep working while the " +
			"sub-agent runs — collect each async result with delegate_await BEFORE you " +
			"give your final answer (un-awaited sub-agents are cancelled when your run " +
			"ends). Prefer reusing an existing named `agent` (roster slug) whose role fits " +
			"over inventing an ad-hoc one. Optionally pick the sub-agent's model (otherwise " +
			"the daemon default) and/or its routing task type (defaults to \"delegate\"); a " +
			"configured routing chain for that task type provides the fallback models.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the sub-agent. Include all context it needs; it does not see this conversation."
    },
    "model": {
      "type": "string",
      "description": "Optional model id for the sub-agent (e.g. a cheaper or stronger model than the lead). Omit to use the daemon default."
    },
    "task_type": {
      "type": "string",
      "description": "Optional routing task type for the sub-agent (e.g. \"code\", \"plan\"); its configured model chain supplies the fallbacks. Defaults to \"delegate\"."
    },
    "agent": {
      "type": "string",
      "description": "Optional named agent (roster slug) to run the sub-agent AS: its soul becomes the sub-agent's identity and its model/task type/cost ceiling apply as defaults. Explicit model/task_type here still win."
    },
    "async": {
      "type": "boolean",
      "description": "When true, return immediately with a spawn_id while the sub-agent runs in the background; collect its result later with delegate_await. Default false (wait for the result)."
    }
  },
  "required": ["task"]
}`),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectCompensable,
			PredictedEffects: []string{
				"Spawn a governed sub-agent run with its own tool loop, budget, and journal correlation.",
				"Async mode may keep the child run active after the delegate call returns until collected or cancelled.",
			},
			AffectedResources: []string{"sub-agent run tree", "model-provider budget", "tools invoked by the delegated child"},
			RollbackNotes:     "Cancel unneeded async children or compensate any child tool effects through their own audit trail; completed model spend cannot be recovered.",
			Confidence:        0.75,
		},
	}
}

// Invoke runs a delegation on behalf of the lead agent.
func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (toolapi.Result, error) {
	var in struct {
		Task     string `json:"task"`
		Model    string `json:"model"`
		TaskType string `json:"task_type"`
		Agent    string `json:"agent"`
		Async    bool   `json:"async"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return toolapi.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	req := Request{Task: in.Task, Model: in.Model, TaskType: in.TaskType, AgentRef: in.Agent}
	if in.Async {
		if t.Spawn == nil {
			return toolapi.Result{Output: "sub-agent spawner not wired", IsError: true}, nil
		}
		id, err := t.Spawn(ctx, req)
		if err != nil {
			return toolapi.Result{Output: "delegation failed: " + err.Error(), IsError: true}, nil
		}
		return toolapi.Result{Output: fmt.Sprintf("spawned sub-agent %s — it is working in the background. Collect its result with delegate_await {\"spawn_id\":%q} before your final answer.", id, id)}, nil
	}
	if t.Run == nil {
		return toolapi.Result{Output: "sub-agent runner not wired", IsError: true}, nil
	}
	out, err := t.Run(ctx, req)
	if err != nil {
		// Surface as a tool error so the lead agent can adapt, not crash.
		return toolapi.Result{Output: "delegation failed: " + err.Error(), IsError: true}, nil
	}
	return toolapi.Result{Output: out}, nil
}

// AwaitTool is the in-process `delegate_await` tool (M881): the collect half
// of async delegation. Its awaiter is wired to the delegation engine after
// the kernel is constructed.
type AwaitTool struct {
	// Await blocks until the spawned delegation finishes and returns its
	// result exactly once.
	Await func(ctx context.Context, spawnID string) (toolapi.Result, error)
}

// NewAwaitTool builds the `delegate_await` tool.
func NewAwaitTool() *AwaitTool { return &AwaitTool{} }

// Definition describes the `delegate_await` tool.
func (t *AwaitTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name: "delegate_await",
		// Collecting an async delegation's result is the same axis as spawning
		// it (M881) — no new capability, it inherits the delegate grant.
		Capability: toolapi.ToolCapability{Name: string(edict.CapDelegate)},
		Description: "Wait for an async delegation (delegate with async=true) to finish and " +
			"return its result. Call it once per spawn_id; issue several delegate_await calls " +
			"in one turn to collect a whole fan-out. If it reports the sub-agent is still " +
			"running, call it again. A result can be collected exactly once.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "spawn_id": {
      "type": "string",
      "description": "The spawn_id returned by delegate(async=true)."
    }
  },
  "required": ["spawn_id"]
}`),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectReversible,
			PredictedEffects: []string{
				"Wait for and collect one previously spawned async sub-agent result.",
			},
			AffectedResources: []string{"async sub-agent result handle"},
			RollbackNotes:     "Collection consumes the local handle; the result remains in the journal and can be inspected there.",
			Confidence:        0.9,
		},
	}
}

// Invoke collects a spawned delegation's result.
func (t *AwaitTool) Invoke(ctx context.Context, input json.RawMessage) (toolapi.Result, error) {
	var in struct {
		SpawnID string `json:"spawn_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return toolapi.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if t.Await == nil {
		return toolapi.Result{Output: "sub-agent awaiter not wired", IsError: true}, nil
	}
	return t.Await(ctx, in.SpawnID)
}

// spawnHandle tracks one asynchronously delegated sub-agent (M881): the child
// runs on its own goroutine while the lead keeps working; the lead collects
// the result via delegate_await. Guarded by the service's mutex except done
// (close-once signal) and the result fields, which are written exactly once
// before done is closed and read only after it is.
type spawnHandle struct {
	parentCorr string
	rootCorr   string
	cancel     context.CancelFunc
	done       chan struct{}
	answer     string
	err        error
}
