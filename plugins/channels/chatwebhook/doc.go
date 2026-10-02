// SPDX-License-Identifier: MIT

// Package chatwebhook is a two-way channel for chat platforms whose integration
// model is "incoming webhook out + a webhook POST in": Google Chat and
// Mattermost. Outbound (and proactive Pulse briefs) post to the configured
// incoming-webhook URL; inbound arrives as a webhook the platform POSTs to this
// channel's Addr+Path. Messages from allowlisted senders drive the agent and the
// reply is posted back via the same outbound webhook (async ack-then-reply, so a
// long agent run never times the inbound request out).
//
// This supersedes the outbound-only Google Chat / Mattermost entries in the push
// family when an inbound Addr is configured. An empty allowlist is fail-closed.
package chatwebhook
