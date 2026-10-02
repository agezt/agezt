// SPDX-License-Identifier: MIT

// Package slack is an in-process duplex Channel (SPEC-04 §1) over the Slack
// platform, using net/http + crypto/hmac only — no external dependency. Unlike
// Telegram (which long-polls), Slack pushes events, so the channel SERVES an
// Events API endpoint (POST /slack/events) for inbound and POSTs chat.postMessage
// for outbound. Inbound is verified with Slack's HMAC-SHA256 request signature
// (signing secret) and a timestamp freshness window (replay protection); a fast
// 200 ACK is returned and the agent runs asynchronously, posting its reply when
// done (the standard Slack pattern — Slack retries if not ACKed within 3s).
//
// Security (SPEC-04 §1.7): inbound is an injection surface. The signature gates
// authenticity (only Slack, with the shared secret, can deliver events); an
// Allowlist of channel ids gates who may drive the agent; bot/self messages are
// ignored so the agent never loops on its own replies. Inbound text is data, and
// the agent's tool calls still pass through Edict.
package slack
