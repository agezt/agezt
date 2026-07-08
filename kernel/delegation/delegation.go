// SPDX-License-Identifier: MIT

// Package delegation provides sub-agent orchestration for the agent loop:
// sync/async delegate tools, spawn bookkeeping, depth tracking, and ancestry
// helpers. It is extracted from kernel/runtime to narrow the composition root.
package delegation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// DepthKey is a context key for tracking sub-agent nesting depth.
type DepthKey struct{}

// DepthFromCtx returns the current sub-agent nesting depth from ctx, or 0.
func DepthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(DepthKey{}).(int); ok {
		return v
	}
	return 0
}

// WithDepth returns a new context with the given sub-agent nesting depth.
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, DepthKey{}, depth)
}

// SystemPrompt is the system message used for all sub-agent runs.
const SystemPrompt = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."

// SpawnHandle is the bookkeeping record for an async sub-agent spawn.
type SpawnHandle struct {
	SpawnID  string `json:"spawn_id"`
	ChildCorr string `json:"child_corr"`
	ParentTask string `json:"parent_corr"`
	CreatedMS int64  `json:"created_ms"`
	ToolName  string `json:"tool_name,omitempty"`
	AgentRef  string `json:"agent_ref,omitempty"`
}

// Prep holds the prepared execution context for a sub-agent run.
type Prep struct {
	Ctx       context.Context
	Corr      string
	Task      string
	Model     string
	TaskType  string
	AgentRef  string
	System    string
	Depth     int
	ParentCorr string
	CreatedMS int64
	Async     bool
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

// Definition implements agent.Tool.
func (t *SubAgentTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "delegate",
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

// Invoke implements agent.Tool by calling the synchronous runner.
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

// Definition implements agent.Tool.
func (t *SubAgentAwaitTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "delegate_await",
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

// Invoke implements agent.Tool by calling the await runner.
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

// SpawnLink extracts child and parent correlation IDs from a subagent.spawned event payload.
func SpawnLink(payload json.RawMessage) (child, parent string) {
	var ev struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.Child, ev.Parent
	}
	return "", ""
}

// BudgetCostMicrocents extracts the cost_microcents from a budget.consumed event payload.
func BudgetCostMicrocents(payload json.RawMessage) int64 {
	var ev struct {
		CostMicrocents int64 `json:"cost_microcents"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.CostMicrocents
	}
	return 0
}

// KeyedModelChain resolves a sub-agent's model from an optional override, a
// model chain, an availability function, and a default. Returns the resolved
// model and the remaining chain.
func KeyedModelChain(subModel string, modelChain []string, avail func(string) bool, def string) (string, []string) {
	if subModel != "" {
		// Explicit override — use as-is, shift chain.
		rest := modelChain
		if len(rest) > 0 && rest[0] == subModel {
			rest = rest[1:]
		}
		return subModel, rest
	}
	if len(modelChain) == 0 {
		return def, nil
	}
	for i, m := range modelChain {
		if avail(m) {
			return m, modelChain[i+1:]
		}
	}
	return def, nil
}

// AppendUniqueStrings appends values that are not already present in the slice.
func AppendUniqueStrings(in []string, values ...string) []string {
	for _, v := range values {
		in = AppendUniqueString(in, v)
	}
	return in
}

// AppendUniqueString appends a value if not already present.
func AppendUniqueString(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}

// ValidateSpawnTask checks that a spawn task is non-empty and not too long.
func ValidateSpawnTask(task string) error {
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("sub-agent task is empty")
	}
	if len(task) > 10000 {
		return fmt.Errorf("sub-agent task too long (%d chars, max 10000)", len(task))
	}
	return nil
}

// FormatDuration formats a duration for sub-agent result reporting.
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
}
