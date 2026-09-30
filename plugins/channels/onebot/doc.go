// SPDX-License-Identifier: MIT

// Package onebot is a two-way channel for any OneBot v11-compatible gateway —
// the de-facto standard HTTP protocol behind QQ bots (go-cqhttp, NapCat,
// Lagrange) and unofficial WeChat bridges (wcf / wechatbot). QQ's personal
// accounts and WeChat have no first-party bot API, so a self-hosted OneBot
// gateway is the realistic path (the same "bring a local gateway" model as the
// WhatsApp WAHA channel). One engine backs both: Config.Kind ("qq" / "wechat")
// only sets the channel name (cf. the IRC channel backing Twitch).
//
// Inbound: the gateway POSTs message events to this channel's Addr+Path
// (optionally HMAC-SHA1 signed via X-Signature). Outbound + replies call the
// gateway's HTTP API (/send_msg). An empty allowlist is fail-closed; without an
// Addr the channel is send-only.
package onebot
