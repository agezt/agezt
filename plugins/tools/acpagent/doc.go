// SPDX-License-Identifier: MIT

// Package acpagent is the in-process ACP-client bridge tool (SPEC-15 §3, the
// inverse of kernel/acp's server): it delegates a task to an *external* ACP
// agent (Claude Code, Codex, Gemini CLI, or any agent that speaks the Agent
// Client Protocol) by spawning it as a subprocess and driving it over JSON-RPC
// 2.0 on stdio — initialize → session/new → session/prompt — relaying the
// agent's streamed message back as the tool result. This lets a Agezt run
// orchestrate another agent as a governed step.
//
// The agent command is configured by the operator (AGEZT_ACP_AGENT_CMD); unset
// → the tool is not registered. The external agent runs with the workspace as
// its session cwd. Because the spawn has real side effects (the external agent
// can edit files / run commands in its own sandbox), the tool is gated Ask-first
// by Edict (the acp_agent capability), like the coding bridge.
package acpagent
