// SPDX-License-Identifier: MIT

// Package vertex is the in-process Google Vertex AI Provider.
//
// **Scope (M1.n):** service-account OAuth + Gemini generateContent
// body shape on the regional aiplatform.googleapis.com endpoint.
// Anthropic-on-Vertex (`@ai-sdk/google-vertex/anthropic`, which uses
// the `:rawPredict` endpoint with the Anthropic Messages body) and
// streaming land in M1.n.x.
//
// Wire (SPEC-15):
//
//	POST https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent
//	Authorization: Bearer {oauth_access_token}
//	Content-Type: application/json
//
// Auth: see auth.go — JWT-bearer flow against the service account's
// token_uri, with a small in-package cache.
//
// Body shape is identical to plugins/providers/google (Generative
// Language API). We duplicate the encoder/decoder rather than reuse
// google's unexported helpers — Vertex evolves independently and
// the duplication is contained.
package vertex
