package delegation

// Provenance: SPDX-License-Identifier: MIT kernel/delegation shared sub-agent
//             vocabulary: the depth context key and the sub-agent system prompt.
//             The delegate / delegate_await TOOLS live in kernel/runtime
//             (subagent_tool.go); the unwired duplicates of them that sat here
//             (SubAgentTool, SubAgentAwaitTool, Prep, SpawnHandle) were removed
//             in 2026-10 (architecture W0.5).

// DepthKey is a context key for tracking sub-agent nesting depth.
type DepthKey struct{}

// SystemPrompt is the system message used for all sub-agent runs.
const SystemPrompt = "You are a focused sub-agent spawned to complete ONE delegated task. " +
	"Work autonomously with the tools available, then report a concise, self-contained " +
	"result the lead agent can use directly. Do not ask clarifying questions; make a " +
	"reasonable assumption and state it."
