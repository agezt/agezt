// SPDX-License-Identifier: MIT

// Package acpclient speaks the Agent Client Protocol (SPEC-15 §3) as the
// CLIENT: it drives an external ACP agent (Gemini CLI, Claude Code's adapter,
// Codex, …) over JSON-RPC 2.0 on stdio — initialize → session/new →
// session/prompt — and relays the agent's streamed agent_message_chunk
// updates.
//
// It lives in the PLATFORM layer (L2) because it is infrastructure with no
// domain knowledge: it speaks a protocol over an io.Reader/io.Writer and
// knows nothing about Agezt's agents, roster or journal. That placement is
// what lets a plugin drive an external ACP agent — a plugin may reach the
// contract and the platform layers, and nothing else.
//
// The JSON-RPC envelope and the message framing below are shared with the
// daemon's own ACP SERVER (kernel/acp, the inbound adapter an IDE drives) —
// both ends of the protocol speak the same wire, so the wire lives with the
// protocol, here.
package acpclient
