// SPDX-License-Identifier: MIT

// Package whatsapp is an inbound/outbound WhatsApp channel over Meta's WhatsApp
// Cloud API (SPEC-04 §1). An allowlisted WhatsApp number can drive an Agezt agent
// by messaging the business number; the agent replies in the same thread.
// Proactive messages (Pulse briefs, `agt send`) go out via the Graph API.
//
// Inbound has two shapes Meta defines:
//   - GET verification handshake: Meta calls with hub.mode=subscribe,
//     hub.verify_token, hub.challenge — the handler echoes the challenge when the
//     token matches (one-time webhook setup).
//   - POST delivery: Meta posts a JSON envelope (entry[].changes[].value.messages[]).
//     The handler authenticates it with the X-Hub-Signature-256 header
//     (sha256=<hex HMAC-SHA256(appSecret, raw body)>); an empty app secret fails
//     closed, so no unsigned inbound.
//
// WhatsApp has no synchronous reply, so the agent's answer is sent back as a fresh
// Graph API message to the sender (the same path as Send); the webhook returns 200
// promptly. Outbound: POST /{PhoneNumberID}/messages with a Bearer access token;
// long text is split with channel.SplitText.
//
// Security (SPEC-04 §1.7): inbound text is data, never kernel instructions; the
// signature authenticates Meta; an Allowlist of sender numbers gates who may drive
// the agent (fail-closed); a dedup set on the message id guards Meta's retries;
// bodies are length-bounded. The agent's own tool calls still pass through Edict.
package whatsapp
