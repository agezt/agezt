// SPDX-License-Identifier: MIT

// Package google is the in-process Google Gemini Provider, talking to
// the Generative Language API at generativelanguage.googleapis.com.
//
// Wire shape (SPEC-15): the Gemini API is meaningfully different from
// OpenAI's — top-level `contents` instead of `messages`, parts arrays
// instead of strings, `model` role instead of `assistant`, tool
// declarations under `tools[0].functionDeclarations`, tool results
// folded back into a user message as `functionResponse` parts.
//
// Auth: API key passed via the `x-goog-api-key` header (preferred over
// the `?key=...` query param so the key doesn't end up in logs). The
// key comes from one of GOOGLE_API_KEY / GOOGLE_GENERATIVE_AI_API_KEY
// / GEMINI_API_KEY, resolved by plugins/providers/compat.
//
// Vertex AI (service-account OAuth, different base URL) is *not*
// covered here — catalog.FamilyGoogleVertex returns
// ErrFamilyUnsupported from compat until a Vertex adapter ships.
//
// Non-streaming for M1.i; SSE streaming lands when streaming is
// added uniformly across providers.
package google
