// SPDX-License-Identifier: MIT

package feishu

// Provenance: Feishu channel: wire-shape parsers (urlVerification + parseEvent).
//             Code extracted from feishu.go during the Day-119 god-file split.
//             Public API unchanged.

import (
	"encoding/json"
	"strings"
)

// feishuWebhook is a Feishu delivery: the URL-verification handshake that
// arrives on the same route as the event.
//
// The handshake carries the challenge and the token at the top level; the
// event carries the token in its header. One body type holds both, because
// one route carries both, and the contract names one member for it.
//
// The shape is NAMED, not anonymous: the contract
// (kernel/contract/channelhookapi) holds the same shape as a JSON Schema, and
// the plugin compares the two when it is built — so a contract that mistyped a
// field stops the daemon instead of quietly disagreeing with the member that
// reads it.
type feishuWebhook struct {
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Type      string `json:"type"`
	Header    struct {
		Token     string `json:"token"`
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID string `json:"open_id"`
			} `json:"sender_id"`
		} `json:"sender"`
		Message struct {
			MessageID   string `json:"message_id"`
			ChatID      string `json:"chat_id"`
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
		} `json:"message"`
	} `json:"event"`
}

// feishuToken reads the verification token out of a raw body: the top-level
// "token" of the URL-verification handshake, or the event's "header.token".
//
// The token is INSIDE the body, so it has to be read from the raw bytes
// before the body is decoded — and it is read before the door runs, so a
// body is only measured by someone who proved they may send it.
//
// The second answer says whether the body was JSON at all. A body that is not
// JSON carries no token, so there is nothing to verify: the door is what
// refuses it, in the member's own words ("bad json").
func feishuToken(body []byte) (string, bool) {
	var doc struct {
		Type   string `json:"type"`
		Token  string `json:"token"`
		Header struct {
			Token string `json:"token"`
		} `json:"header"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", false
	}
	// The handshake's token is at the top level; the event's is in its header.
	// The two never appear together in a real delivery, but the member chose
	// by the body before, so the probe chooses the same way.
	if doc.Type == "url_verification" {
		return doc.Token, true
	}
	return doc.Header.Token, true
}

// urlVerification detects the one-time challenge POST and returns the
// challenge.
//
// The body arrives already DECODED and already MEASURED — the door
// (handleInbound) holds it to the contract's schema before it gets here — so
// this function only reads the fields it needs.
func urlVerification(w feishuWebhook) (string, bool) {
	if w.Type != "url_verification" || w.Challenge == "" {
		return "", false
	}
	return w.Challenge, true
}

// parseEvent reads an im.message.receive_v1 event (schema 2.0). The message
// content is itself a JSON string ({"text":"…"}). Returns (msg, token, ok).
//
// The body arrives already DECODED and already MEASURED — the door
// (handleInbound) holds it to the contract's schema before it gets here — so
// this function only reads the fields it needs.
func parseEvent(e feishuWebhook) (inbound, string, bool) {
	if e.Header.EventType != "im.message.receive_v1" {
		return inbound{}, e.Header.Token, false
	}
	var content struct {
		Text     string `json:"text"`
		ImageKey string `json:"image_key"`
		FileKey  string `json:"file_key"`
	}
	_ = json.Unmarshal([]byte(e.Event.Message.Content), &content)
	id := e.Event.Message.MessageID
	if id == "" {
		id = e.Header.EventID
	}
	in := inbound{
		sender:    e.Event.Sender.SenderID.OpenID,
		chatID:    e.Event.Message.ChatID,
		text:      strings.TrimSpace(content.Text),
		id:        id,
		messageID: e.Event.Message.MessageID,
	}
	switch e.Event.Message.MessageType {
	case "text":
	case "image":
		in.fileKey, in.mediaType = content.ImageKey, "image"
	case "audio":
		in.fileKey, in.mediaType = content.FileKey, "audio"
	default:
		return inbound{}, e.Header.Token, false
	}
	return in, e.Header.Token, true
}
