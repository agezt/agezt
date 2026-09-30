// SPDX-License-Identifier: MIT

// Package anthropic is the in-process Anthropic Messages-API Provider.
//
// It translates between Agezt's canonical (dialect-free) agent.Message /
// agent.ToolCall / agent.ToolDef shapes and Anthropic's content-block
// format (SPEC-15). Non-streaming for M0.5; streaming lands later.
//
// Auth: api-key via the AGEZT_ANTHROPIC_API_KEY env var (or the constructor
// argument). OAuth/subscription auth lands with the Governor in MVP
// (TASKS P1-PROV-01).
package anthropic
