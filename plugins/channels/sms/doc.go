// SPDX-License-Identifier: MIT

// Package sms is an inbound/outbound SMS channel over Twilio's Programmable
// Messaging API (SPEC-04 §1). An allowlisted phone number can drive an Agezt
// agent by texting the daemon's Twilio number; the agent replies in the same
// thread. Proactive messages (Pulse briefs, `agt send`) go out via Twilio's
// REST API.
//
// Inbound: Twilio POSTs application/x-www-form-urlencoded to the configured
// route (From, Body, MessageSid, …). The handler authenticates the request with
// the X-Twilio-Signature header (base64 HMAC-SHA1 over the request URL + sorted
// POST params, keyed by the account auth token) — empty auth token fails closed,
// so no unsigned inbound. The reply is returned synchronously as TwiML
// (<Response><Message>…</Message></Response>), Twilio's native reply path.
//
// Outbound: POST to /2010-04-01/Accounts/{SID}/Messages.json, form-encoded
// (To/From/Body), HTTP Basic auth (AccountSID:AuthToken). Long replies are split
// with channel.SplitText.
//
// Security (SPEC-04 §1.7): inbound text is data, never kernel instructions; the
// signature authenticates Twilio; an Allowlist of sender numbers gates who may
// drive the agent (fail-closed); a dedup set on MessageSid guards retries; bodies
// are length-bounded. The agent's own tool calls still pass through Edict.
package sms
