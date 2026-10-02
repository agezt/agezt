// SPDX-License-Identifier: MIT

// Package chatgptauth manages the OAuth tokens for the "Sign in with ChatGPT"
// provider — the same subscription auth Codex CLI uses. It owns the PKCE
// constants, the authorize-URL + code/refresh exchanges against auth.openai.com,
// the at-rest token store (one JSON secret in the kernel vault), and extraction
// of the ChatGPT account id from the id_token.
//
// This deliberately reuses the Codex CLI's public OAuth client and the ChatGPT
// backend — an UNOFFICIAL, undocumented path that may break or run afoul of
// OpenAI's terms. It only ever authenticates the operator's own account; the UI
// gates it behind an explicit acknowledgement.
package chatgptauth
