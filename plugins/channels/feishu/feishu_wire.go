// SPDX-License-Identifier: MIT

// Feishu channel: wire-shape parsers (urlVerification + parseEvent).
// Code extracted from feishu.go during the Day-119 god-file split.
// Public API unchanged.
package feishu


import (
	"strings"

	"encoding/json"
)

func urlVerification(body []byte) (string, string, bool) {
	var v struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", "", false
	}
	if v.Type != "url_verification" || v.Challenge == "" {
		return "", "", false
	}
	return v.Challenge, v.Token, true
}

// parseEvent reads an im.message.receive_v1 event (schema 2.0). The message
// content is itself a JSON string ({"text":"…"}). Returns (msg, token, ok).
func parseEvent(body []byte) (inbound, string, bool) {
	var e struct {
		Header struct {
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
	if err := json.Unmarshal(body, &e); err != nil {
		return inbound{}, "", false
	}
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
