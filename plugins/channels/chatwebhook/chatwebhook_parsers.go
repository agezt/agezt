// SPDX-License-Identifier: MIT

package chatwebhook

// Provenance: Package chatwebhook: platform-specific inbound payload parsers
//             (chatwebhookBody + parseMattermost + parseGoogleChat).
//             Extracted from chatwebhook.go during the Day-211 god-file split.
//             Public API unchanged.

import (
	"net/url"
	"strings"
)

// chatwebhookBody is the Google Chat delivery the daemon reads: the event's
// type, the message with its sender, and the space it belongs to.
//
// The member's OTHER dialect — Mattermost's outgoing webhook — is a FORM, not
// a JSON body, so the door does not measure it.
//
// The shape is named in the contract (kernel/contract/channelhookapi), and the
// door measures an arriving body against it before the plugin reads it — so
// this type and the contract's schema must name the SAME fields with the SAME
// types. The member checks that when it is built.
type chatwebhookBody struct {
	Type    string `json:"type"`
	Message struct {
		Name   string `json:"name"`
		Text   string `json:"text"`
		Sender struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
		} `json:"sender"`
	} `json:"message"`
	Space struct {
		Name string `json:"name"`
	} `json:"space"`
}

// parseMattermost reads a Mattermost outgoing-webhook form POST: token, user_name,
// channel_name, text, post_id, trigger_word.
func parseMattermost(body []byte) (inbound, bool) {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return inbound{}, false
	}
	text := vals.Get("text")
	if tw := vals.Get("trigger_word"); tw != "" {
		text = strings.TrimSpace(strings.TrimPrefix(text, tw))
	}
	return inbound{
		sender: vals.Get("user_name"),
		target: vals.Get("channel_name"),
		text:   text,
		id:     vals.Get("post_id"),
	}, true
}

// parseGoogleChat reads a Google Chat app event: {type, message:{name, text,
// sender:{name, displayName, email}}, space:{name}}. Only MESSAGE events are kept.
//
// The body has already been measured against the contract's shape by the door
// — a mistyped field never reaches this function.
func parseGoogleChat(e chatwebhookBody) (inbound, bool) {
	if e.Type != "" && e.Type != "MESSAGE" {
		return inbound{}, false
	}
	sender := e.Message.Sender.Email
	if sender == "" {
		sender = e.Message.Sender.DisplayName
	}
	if sender == "" {
		sender = e.Message.Sender.Name
	}
	return inbound{
		sender: sender,
		target: e.Space.Name,
		text:   e.Message.Text,
		id:     e.Message.Name,
	}, true
}
