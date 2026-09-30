// SPDX-License-Identifier: MIT

// Package telegram is an in-process duplex Channel (SPEC-04 §1) over the
// Telegram Bot API, using net/http only — no external dependency. It
// long-polls getUpdates for inbound messages and POSTs sendMessage for
// outbound. The same Channel interface an out-of-process plugin will satisfy
// later (SPEC-04 §1.6); in-process is the Phase-4 MVP choice, matching how
// memory/pulse run inside the daemon.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Only chat ids on
// the allowlist may drive the agent; everyone else is journaled and ignored.
// Inbound text is passed to the agent as an intent (data), and the agent's
// tool calls still pass through Edict.
package telegram
