// SPDX-License-Identifier: MIT

// Package openairesponses is the provider adapter for "Sign in with ChatGPT":
// it speaks the OpenAI Responses API against the ChatGPT backend
// (https://chatgpt.com/backend-api/codex/responses) using a subscription OAuth
// access token, the same wire Codex CLI uses. It translates AGEZT's
// chat-shaped CompletionRequest to/from Responses items, streams the SSE reply,
// and assembles a single non-streaming CompletionResponse.
//
// This is an UNOFFICIAL, undocumented backend (it requires Codex's own system
// instructions, embedded below) and may break or violate OpenAI's terms — see
// kernel/chatgptauth. The instructions.md file is Codex's prompt, vendored from
// the Apache-2.0 openai/codex repo so the backend accepts our requests.
package openairesponses
