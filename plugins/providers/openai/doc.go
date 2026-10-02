// SPDX-License-Identifier: MIT

// Package openai is the in-process OpenAI Chat Completions Provider.
//
// One adapter covers two compatibility families: FamilyOpenAI (the
// real api.openai.com) and FamilyOpenAICompatible (Groq, DeepSeek,
// Together, OpenRouter, xAI, Fireworks, Cerebras, SambaNova, …). All
// of those expose the same /v1/chat/completions wire shape with
// Bearer-token auth; the only difference is the base URL and the env
// var holding the key, both of which come from the catalog.
//
// Non-streaming for M1.h; streaming lands when SSE-aware Providers
// arrive across the board.
//
// Auth: Bearer <key> via the constructor or the BaseURL/Endpoint
// pair set by plugins/providers/compat.
package openai
