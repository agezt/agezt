// SPDX-License-Identifier: MIT

// Package dingtalk is a two-way DingTalk channel over the enterprise robot
// "outgoing" model. When the robot is @-mentioned, DingTalk POSTs the message to
// this channel's Addr+Path (signed with timestamp+sign headers, verified against
// the robot's secret). Each inbound carries a short-lived `sessionWebhook` URL we
// POST the reply back to — so replies need no token fetch. Proactive briefs /
// `agt send` use the configured custom-robot webhook URL.
//
// An empty allowlist is fail-closed. Without an Addr the channel is send-only.
package dingtalk
