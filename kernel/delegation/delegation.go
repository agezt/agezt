// SPDX-License-Identifier: MIT
//
// kernel/delegation sub-agent tool types (DepthKey, Prep, SubAgentTool,
// SubAgentAwaitTool) + DefaultSubAgentMaxDepth const + SubAgentTool + SubAgentAwaitTool
// Definition/Invoke methods.
// Extracted from delegation.go during Day 211 god-file refactor (#90).
// Public API unchanged.
package delegation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

// DepthKey is a context key for tracking sub-agent nesting depth.
type DepthKey struct{}

// DepthFromCtx returns the current sub-agent nesting depth from ctx, or 0.

// SystemPrompt is the system message used for all sub-agent runs.
const SystemPrompt = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."

// SpawnHandle is the bookkeeping record for an async sub-agent spawn.
type SpawnHandle struct {
	SpawnID    string `json:"spawn_id"`
	ChildCorr  string `json:"child_corr"`
	ParentTask string `json:"parent_corr"`
	CreatedMS  int64  `json:"created_ms"`
	ToolName   string `json:"tool_name,omitempty"`
	AgentRef   string `json:"agent_ref,omitempty"`
}

// Prep holds the prepared execution context for a sub-agent run.
type Prep struct {
	Ctx        context.Context
	Corr       string
	Task       string
	Model      string
	TaskType   string
	AgentRef   string
	System     string
	Depth      int
	ParentCorr string
	CreatedMS  int64
	Async      bool
}

// DefaultSubAgentMaxDepth is the maximum allowed sub-agent nesting.
const DefaultSubAgentMaxDepth = 8

// SubAgentTool is the in-process delegate tool. Its runners are wired
// after the kernel is constructed.
type SubAgentTool struct {
	Run   func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
	Spawn func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
}

// NewSubAgentTool creates a SubAgentTool with nil runners (wired externally).
func NewSubAgentTool() *SubAgentTool { return &SubAgentTool{} }
func (t *SubAgentTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "delegate",
		Capability:  agent.ToolCapability{Name: string(edict.CapDelegate)},
		Description: "Spawns a sub-agent to complete a focused task and waits for its result. The sub-agent gets its own tools, budget, and context window. Use this for independent subtasks that would benefit from parallel reasoning or when the main agent's context is full.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["task"],
			"properties": {
				"task": {"type": "string", "description": "The focused task for the sub-agent"},
				"model": {"type": "string", "description": "Optional model override"},
				"task_type": {"type": "string", "description": "Optional task type hint"},
				"agent_ref": {"type": "string", "description": "Optional named agent profile ref"}
			}
		}`),
	}
}
func (t *SubAgentTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var params struct {
		Task     string `json:"task"`
		Model    string `json:"model,omitempty"`
		TaskType string `json:"task_type,omitempty"`
		AgentRef string `json:"agent_ref,omitempty"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return agent.Result{}, fmt.Errorf("delegate: invalid input: %w", err)
	}
	if params.Task == "" {
		return agent.Result{}, fmt.Errorf("delegate: 'task' is required")
	}
	if t.Run == nil {
		return agent.Result{}, fmt.Errorf("delegate: not initialized")
	}
	out, err := t.Run(ctx, params.Task, params.Model, params.TaskType, params.AgentRef)
	if err != nil {
		return agent.Result{Output: err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: out}, nil
}

// SubAgentAwaitTool is the in-process delegate_await tool.
type SubAgentAwaitTool struct {
	Await func(ctx context.Context, spawnID string) (agent.Result, error)
}

// NewSubAgentAwaitTool creates a SubAgentAwaitTool with nil runner.
func NewSubAgentAwaitTool() *SubAgentAwaitTool { return &SubAgentAwaitTool{} }

func (t *SubAgentAwaitTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "delegate_await",
		Capability:  agent.ToolCapability{Name: string(edict.CapDelegate)},
		Description: "Awaits the result of a previously spawned async sub-agent by its spawn_id. Returns the result once the sub-agent completes.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["spawn_id"],
			"properties": {
				"spawn_id": {"type": "string", "description": "The spawn_id returned by delegate with async=true"}
			}
		}`),
	}
}
func (t *SubAgentAwaitTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var params struct {
		SpawnID string `json:"spawn_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return agent.Result{}, fmt.Errorf("delegate_await: invalid input: %w", err)
	}
	if params.SpawnID == "" {
		return agent.Result{}, fmt.Errorf("delegate_await: 'spawn_id' is required")
	}
	if t.Await == nil {
		return agent.Result{}, fmt.Errorf("delegate_await: not initialized")
	}
	return t.Await(ctx, params.SpawnID)
}
