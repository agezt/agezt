// SPDX-License-Identifier: MIT

// Package signal is an in-process duplex Channel (SPEC-04 §1) for Signal, talking
// to a signal-cli-rest-api server (github.com/bbernhard/signal-cli-rest-api) over
// net/http only — no external dependency. It long-polls GET /v1/receive/{number}
// for inbound messages and POSTs /v2/send for outbound, mirroring how the Matrix
// channel long-polls /sync. signal-cli-rest-api wraps signal-cli behind a small
// HTTP API the operator runs locally (or in their own network), so the URL is
// operator-pinned: there is no SSRF surface, just as with the Home Assistant tool.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Only senders on the
// allowlist may drive the agent; everyone else is journaled and ignored. The
// account's OWN number is skipped so a reply never re-enters the loop. Inbound
// text is passed to the agent as an intent (data), and the agent's tool calls
// still pass through Edict.
//
// Scope: text messages. Attachments (inbound images → data: URL for vision) are a
// deliberate follow-up, kept out so this first cut stays small and correct.
package signal
