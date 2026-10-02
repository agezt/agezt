// SPDX-License-Identifier: MIT

// Package discord is an in-process duplex Channel (SPEC-04 §1) over Discord,
// using net/http + crypto/ed25519 only — no external dependency, no Gateway
// WebSocket. Free-form Discord messages require the Gateway (a persistent
// WebSocket), which would pull in a dependency; instead this channel drives the
// agent through Discord's HTTP **Interactions** endpoint (a slash command such
// as `/agezt prompt:<text>`). Discord POSTs the interaction to a URL the channel
// SERVES (POST /discord/interactions); the channel verifies Discord's Ed25519
// request signature, ACKs with a DEFERRED response within 3s ("Agezt is
// thinking…"), runs the agent asynchronously, and delivers the answer with a
// follow-up webhook message. Outbound briefs (Pulse) post via the bot token to
// channels/{id}/messages.
//
// This is the same channel.Channel shape as Telegram (long-poll) and Slack
// (HMAC webhook) — only the transport and signature scheme differ, proving the
// abstraction generalizes: Telegram pulls, Slack/Discord push, Slack signs with
// HMAC-SHA256, Discord signs with Ed25519.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. The Ed25519 signature
// gates authenticity (only Discord, holding the app's private key, can deliver a
// valid interaction); an empty/invalid public key fails closed. An Allowlist of
// channel ids gates who may drive the agent. Inbound text is data, and the
// agent's tool calls still pass through Edict.
package discord
