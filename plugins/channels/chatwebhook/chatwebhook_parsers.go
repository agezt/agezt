// SPDX-License-Identifier: MIT

// Package chatwebhook: platform-specific inbound payload parsers (parseInbound
// dispatcher + parseMattermost + parseGoogleChat). Extracted from
// chatwebhook.go during the Day-211 god-file split. Public API unchanged.
package chatwebhook


import (
	"encoding/json"
	"net/url"
	"strings"
)
func parseInbound(kind string, body []byte) (inbound, bool) {
	if kind == KindMattermost {
		return parseMattermost(body)
	}
	return parseGoogleChat(body)
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
func parseGoogleChat(body []byte) (inbound, bool) {
	var e struct {
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
	if err := json.Unmarshal(body, &e); err != nil {
		return inbound{}, false
	}
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
