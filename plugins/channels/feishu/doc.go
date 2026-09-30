// SPDX-License-Identifier: MIT

// Package feishu is a two-way Feishu / Lark channel over an app event
// subscription. Feishu POSTs events to this channel's Addr+Path: a one-time
// url_verification challenge (echoed back) and im.message.receive_v1 message
// events (verified against the app's verification token). Replies are sent via
// the IM API using a tenant_access_token fetched from the app id/secret (cached
// until expiry). An empty allowlist is fail-closed; without an Addr the channel
// is send-only (briefs to a configured chat).
package feishu
