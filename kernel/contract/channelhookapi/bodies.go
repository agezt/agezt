// SPDX-License-Identifier: MIT

package channelhookapi

// The vendor members' bodies: each a JSON Schema, written out as the
// contract states them. The shapes are the VENDORS' — Slack's event
// envelope, Discord's interaction, Meta's WhatsApp payload — so the
// contract names the fields the vendor names and may not rename one.

// Slack's Events API envelope: the handshake's type and
// challenge, and the event itself.
const slackBody = `{"type":"object","additionalProperties":true,"properties":{"type":{"type":"string"},"challenge":{"type":"string"},"event":{"type":"object","additionalProperties":true,"properties":{"type":{"type":"string"},"channel":{"type":"string"},"user":{"type":"string"},"text":{"type":"string"},"ts":{"type":"string"},"thread_ts":{"type":"string"},"bot_id":{"type":"string"},"subtype":{"type":"string"},"files":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"url_private":{"type":"string"},"mimetype":{"type":"string"},"name":{"type":"string"}}}}}}}}`

// Discord's interaction envelope: the interaction itself, the
// command's data (with its options and the objects they resolve to), and the
// member or user who sent it.
//
// The option's value is "any": Discord sends a string for a text option and a
// number for a number one, and the plugin reads the value only when the
// option's TYPE says which.
const discordBody = `{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"type":{"type":"number"},"token":{"type":"string"},"channel_id":{"type":"string"},"data":{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"},"options":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"},"type":{"type":"number"},"value":{"type":"any"}}}},"resolved":{"type":"object","additionalProperties":true,"properties":{"attachments":{"type":"object","additionalProperties":{"type":"object","additionalProperties":true,"properties":{"url":{"type":"string"},"content_type":{"type":"string"},"filename":{"type":"string"}}}}}}}},"member":{"type":"object","additionalProperties":true,"properties":{"user":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"username":{"type":"string"}}}}},"user":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"username":{"type":"string"}}}}}`

// Meta's WhatsApp Cloud API webhook payload: the entries, the
// changes inside them, and the messages the change carries.
const whatsappBody = `{"type":"object","additionalProperties":true,"properties":{"entry":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"changes":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"value":{"type":"object","additionalProperties":true,"properties":{"messages":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"from":{"type":"string"},"id":{"type":"string"},"type":{"type":"string"},"text":{"type":"object","additionalProperties":true,"properties":{"body":{"type":"string"}}},"audio":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"}}},"voice":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"}}},"image":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"caption":{"type":"string"}}}}}}}}}}}}}}}}`

// LINE's webhook payload: the events, each with its source, its
// message and the token that answers it without a reply endpoint.
const lineBody = `{"type":"object","additionalProperties":true,"properties":{"events":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"type":{"type":"string"},"replyToken":{"type":"string"},"source":{"type":"object","additionalProperties":true,"properties":{"userId":{"type":"string"},"groupId":{"type":"string"}}},"message":{"type":"object","additionalProperties":true,"properties":{"type":{"type":"string"},"id":{"type":"string"},"text":{"type":"string"}}}}}}}}`

// the BlueBubbles bridge's "new-message" webhook: the event's
// type and the data it carries.
const imessageBody = `{"type":"object","additionalProperties":true,"properties":{"type":{"type":"string"},"data":{"type":"object","additionalProperties":true,"properties":{"guid":{"type":"string"},"text":{"type":"string"},"isFromMe":{"type":"boolean"},"handle":{"type":"object","additionalProperties":true,"properties":{"address":{"type":"string"}}},"chats":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"guid":{"type":"string"}}}},"attachments":{"type":"array","items":{"type":"object","additionalProperties":true,"properties":{"guid":{"type":"string"},"mimeType":{"type":"string"},"transferName":{"type":"string"}}}}}}}}`

// a Zalo OA event: the event's name, who sent it, the message and
// the timestamp the signature covers.
const zaloBody = `{"type":"object","additionalProperties":true,"properties":{"event_name":{"type":"string"},"sender":{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"}}},"message":{"type":"object","additionalProperties":true,"properties":{"msg_id":{"type":"string"},"text":{"type":"string"}}},"timestamp":{"type":"string"}}}`
