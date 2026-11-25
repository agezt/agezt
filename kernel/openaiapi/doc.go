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
//
// The surface has ONE home: kernel/contract/openaiapi names every route —
// path, method and the request body it takes — and the server registers from
// that table, so a route in the contract without a handler stops the daemon at
// boot. An arriving JSON body is measured against the contract's schema
// before the handler reads it, with the operation registry's own words
// ("operation chat_completions input: $.model has wrong type: got number,
// want string") inside the OpenAI error envelope every OpenAI client reads.
// The OpenAI field names are the vendor's and are not changed here.
package openaiapi
