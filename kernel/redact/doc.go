// SPDX-License-Identifier: MIT

// Package redact scrubs secrets from text before it is persisted. The kernel's
// journal is append-only and hash-chained: anything written there is permanent.
// A secret that reaches an event payload — an API key echoed in a tool's output,
// a token pasted into a prompt, a credential in an HTTP response — would be
// recorded forever. This package is the chokepoint that prevents that
// (SPEC-06 / ROADMAP "redaction must work before Initiative can act
// autonomously").
//
// It redacts on two signals:
//
//   - Literals: exact secret values the daemon knows (e.g. the configured
//     provider keys from the creds store). Scrubbed wherever they appear, even
//     mid-string and nested.
//   - Patterns: high-confidence secret *formats* (OpenAI/Anthropic `sk-…`, AWS
//     `AKIA…`, GitHub `ghp_…`, Slack `xox…`/`xapp-…`, Telegram bot tokens,
//     Groq `gsk_…`, xAI `xai-…`, Perplexity `pplx-…`, Fireworks `fw_…`,
//     Google `AIza…`, bearer tokens, JWTs, PEM private-key blocks). These catch
//     secrets the daemon was never told about.
//
// Redaction is a pure, deterministic function of (input, literal set): the same
// input always yields the same output, so a redacted payload hashes stably and
// replay is unaffected (the journal already holds the redacted form). Patterns
// are deliberately specific to avoid corrupting legitimate data.
package redact
