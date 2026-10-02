// SPDX-License-Identifier: MIT

// Package wecom is a two-way WeCom (WeChat Work / 企业微信) channel over an app's
// encrypted callback. WeCom verifies the callback URL with a GET (msg_signature +
// echostr), then POSTs AES-256-CBC-encrypted XML messages. We verify the SHA-1
// message signature, decrypt (WXBizMsgCrypt scheme), and reply via the app
// message-send API using an access_token fetched from the corp id/secret (cached
// until expiry). An empty allowlist is fail-closed; without an Addr the channel
// is send-only.
package wecom
