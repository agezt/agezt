// SPDX-License-Identifier: MIT

// Package openaiapi serves an OpenAI-compatible HTTP surface (ROADMAP P7-API-01,
// SPEC-15 §3): POST /v1/chat/completions, POST /v1/responses, GET /v1/models and
// GET /v1/models/{id}, so any OpenAI client, SDK, or IDE can drive Agezt as if it
// were OpenAI. Every request runs
// through the same kernel tool-loop as `agt run` — so it passes through Edict,
// the journal, and the budget exactly like any other run. It is NOT a
// governance backdoor (P7-API-02 DoD).
//
// The mapping is deliberate and lossy-by-design: OpenAI `messages[]` collapse
// into one Agezt intent (Agezt is an agent, not a raw completion endpoint —
// the configured provider/model and system prompt are the kernel's, not the
// caller's). The caller's `model` field is echoed back but routing is the
// Governor's job. Streaming maps the kernel's llm.token events to OpenAI
// chat.completion.chunk SSE frames.
//
// Security (SPEC-06): loopback-bound by the operator, token-authed on every
// request (Authorization: Bearer <token>). Empty token fails closed.
package openaiapi
