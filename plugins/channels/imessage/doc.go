// SPDX-License-Identifier: MIT

// Package imessage is a two-way iMessage channel over a self-hosted BlueBubbles
// server (https://bluebubbles.app) running on a Mac. BlueBubbles exposes a REST
// API for sending and POSTs a webhook for each incoming message — exactly the
// same shape as the WhatsApp gateway (a local REST gateway with an inbound
// webhook), so this reuses that proven model.
//
// Outbound: POST /api/v1/message/text?password=… with {chatGuid, tempGuid,
// message, method}. Inbound: point the BlueBubbles server's webhook at this
// channel's Addr+Path; "new-message" events from allowlisted senders drive the
// agent and the reply is sent back into the same chat. An empty allowlist is
// fail-closed (outbound-only). Without an Addr the channel is send-only
// (notifications, briefs, `agt send`).
package imessage
