// SPDX-License-Identifier: MIT

// Package channelhookapi is the one home of the channel webhook family's
// inbound bodies.
//
// The family is the set of plugins under plugins/channels that serve an
// inbound webhook: each opens its OWN http.ServeMux for one route and reads
// the body into whatever struct it happened to use — nine of the thirteen JSON
// bodies into an **anonymous** struct. So what happened to a mistyped body was
// decided by Go's encoding/json, and the answer was the plugin's own plain
// text, never the registry's words:
//
//	POST /webhook {"text":42,…}    → 400 "bad json"
//	POST /webhook {"text":null,…}  → 400 "channel_id and text are required"
//
// and a null was often accepted at all: four of the six nulls measured on the
// generic member were answered 200.
//
// This package holds the family's inbound vocabulary as data: every member's
// body is a JSON Schema, and the door measures an arriving body against it
// before the plugin reads it.
//
// Three rules keep the schemas the operation registry's shape:
//
//  1. A schema names TYPES, not presence. The door checks types; a missing
//     field stays the plugin's business ("channel_id and text are required").
//  2. A schema allows an undeclared field. A body is the stable SUBSET of
//     what the daemon reads — and a vendor's body carries fields Agezt never
//     looks at.
//  3. The type words are the registry's: an integer is a "number", a boolean
//     is a "boolean".
//
// A member's body is the VENDOR's shape (Slack's event envelope, Discord's
// interaction, …), so the contract names the fields the vendor names and may
// not rename one. Only the generic webhook member's body is Agezt's own.
//
// One type word is the family's own: **"any"**. It names a field the daemon
// reads as RAW JSON — a field whose value may be any JSON value, and which
// the daemon therefore cannot measure. Discord's slash-command option value
// is one: Discord sends a string for a text option and a number for a number
// one, and the plugin reads the value only when the option's TYPE says which.
// The door does not measure such a field, and the shape guard holds the two
// together: "any" is the only word a raw field may carry, and only a raw field
// may carry it.
package channelhookapi

import "encoding/json"

// Member is one member of the channel webhook family: a plugin that serves an
// inbound webhook.
type Member struct {
	// ID identifies the member ("webhook"). It is the operation name the
	// door's refusal carries: "operation webhook input: …".
	ID string
	// Body is the JSON Schema of the member's inbound body.
	Body json.RawMessage
}

// Member IDs of the family's members that take a JSON body.
const (
	// MemberWebhook is the generic, vendor-neutral webhook member: any HTTP
	// caller may post Agezt's own inbound shape to it.
	MemberWebhook = "webhook"
	// MemberSlack is Slack's Events API envelope.
	MemberSlack = "slack"
	// MemberDiscord is Discord's interaction envelope.
	MemberDiscord = "discord"
	// MemberWhatsApp is Meta's WhatsApp Cloud API webhook payload.
	MemberWhatsApp = "whatsapp"
	// MemberLine is LINE's webhook payload.
	MemberLine = "line"
	// MemberIMessage is the BlueBubbles bridge's "new-message" webhook.
	MemberIMessage = "imessage"
	// MemberZalo is a Zalo OA event.
	MemberZalo = "zalo"
)

// webhookBody is the generic member's inbound body.
//
// The shape is Agezt's own (not a vendor's): the channel the message is for,
// who sent it, the text, an optional client message id, an optional client
// timestamp and optional image attachments.
const webhookBody = `{"type":"object","additionalProperties":true,"properties":{` +
	`"channel_id":{"type":"string"},` +
	`"sender":{"type":"string"},` +
	`"text":{"type":"string"},` +
	`"id":{"type":"string"},` +
	`"ts_ms":{"type":"number"},` +
	`"images":{"type":"array","items":{"type":"string"}}}}`

// Members returns the family's member table.
func Members() []Member {
	return []Member{
		{ID: MemberWebhook, Body: json.RawMessage(webhookBody)},
		{ID: MemberSlack, Body: json.RawMessage(slackBody)},
		{ID: MemberDiscord, Body: json.RawMessage(discordBody)},
		{ID: MemberWhatsApp, Body: json.RawMessage(whatsappBody)},
		{ID: MemberLine, Body: json.RawMessage(lineBody)},
		{ID: MemberIMessage, Body: json.RawMessage(imessageBody)},
		{ID: MemberZalo, Body: json.RawMessage(zaloBody)},
	}
}

// Body returns the JSON Schema of a member's inbound body.
//
// It answers false for a member the contract does not know — and for a member
// that takes no JSON body, so a caller can tell "this member has no body" from
// "this member is not in the contract".
func Body(memberID string) (json.RawMessage, bool) {
	for _, m := range Members() {
		if m.ID != memberID {
			continue
		}
		if len(m.Body) == 0 {
			return nil, false
		}
		return m.Body, true
	}
	return nil, false
}
