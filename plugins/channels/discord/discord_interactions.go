// SPDX-License-Identifier: MIT

package discord

// Discord interaction data types + extractors: discordInteraction +
// discordData + discordOption + discordResolved + discordAttachment +
// discordMember + discordUser + senderID + imageAttachments +
// audioAttachments + text + optionTypeString + optionTypeAttachment
// consts. Carved out of discord.go during the Day 170 god-file split so
// the main file can stay focused on lifecycle + handlers + outbound +
// emit.
// Public API unchanged.

import (
	"encoding/json"
	"strings"
)
type discordInteraction struct {
	ID        string         `json:"id"`
	Type      int            `json:"type"`
	Token     string         `json:"token"` // follow-up webhook token
	ChannelID string         `json:"channel_id"`
	Data      *discordData   `json:"data"`
	Member    *discordMember `json:"member"` // present in a guild
	User      *discordUser   `json:"user"`   // present in a DM
}

type discordData struct {
	Name     string           `json:"name"`
	Options  []discordOption  `json:"options"`
	Resolved *discordResolved `json:"resolved"`
}

type discordOption struct {
	Name  string          `json:"name"`
	Type  int             `json:"type"`
	Value json.RawMessage `json:"value"`
}

// discordResolved holds the full objects referenced by option values. An
// ATTACHMENT option's value is an attachment id that indexes Attachments.
type discordResolved struct {
	Attachments map[string]discordAttachment `json:"attachments"`
}

type discordAttachment struct {
	URL         string `json:"url"`          // public CDN url, no auth needed
	ContentType string `json:"content_type"` // e.g. "image/png"
	Filename    string `json:"filename"`
}

type discordMember struct {
	User *discordUser `json:"user"`
}

type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (in discordInteraction) senderID() string {
	if in.Member != nil && in.Member.User != nil {
		return in.Member.User.ID
	}
	if in.User != nil {
		return in.User.ID
	}
	return ""
}

// optionTypeString is Discord's APPLICATION_COMMAND option type for a STRING
// (https://discord.com/developers/docs/interactions/...): only these carry a
// free-text value. Sub-command / group options (types 1, 2) nest their own
// options and must not be mistaken for the prompt.
const optionTypeString = 3

// optionTypeAttachment is Discord's APPLICATION_COMMAND option type for an
// ATTACHMENT: the option value is an attachment id resolved via
// data.resolved.attachments (M249).
const optionTypeAttachment = 11

// imageAttachments returns the CDN urls + content types of attachment options
// that resolve to an image. Only options the registered command declared as
// ATTACHMENT are considered; a non-image attachment is ignored.
func (in discordInteraction) imageAttachments() []discordAttachment {
	if in.Data == nil || in.Data.Resolved == nil {
		return nil
	}
	var out []discordAttachment
	for _, o := range in.Data.Options {
		if o.Type != optionTypeAttachment {
			continue
		}
		var id string
		if err := json.Unmarshal(o.Value, &id); err != nil || id == "" {
			continue
		}
		att, ok := in.Data.Resolved.Attachments[id]
		if !ok || att.URL == "" || !strings.HasPrefix(att.ContentType, "image/") {
			continue
		}
		out = append(out, att)
	}
	return out
}

// audioAttachments returns attachment options that resolve to audio (a voice
// message or an uploaded clip). They're transcribed by the ambient STT path when
// AGEZT_STT_* is configured, so a user can talk to the agent on Discord.
func (in discordInteraction) audioAttachments() []discordAttachment {
	if in.Data == nil || in.Data.Resolved == nil {
		return nil
	}
	var out []discordAttachment
	for _, o := range in.Data.Options {
		if o.Type != optionTypeAttachment {
			continue
		}
		var id string
		if err := json.Unmarshal(o.Value, &id); err != nil || id == "" {
			continue
		}
		att, ok := in.Data.Resolved.Attachments[id]
		if !ok || att.URL == "" || !strings.HasPrefix(att.ContentType, "audio/") {
			continue
		}
		out = append(out, att)
	}
	return out
}

// text returns the slash command's prompt. It prefers the option explicitly
// named "prompt" (the registered command's text field) and only considers STRING
// options, so a reordered or additional option can't silently feed the agent the
// wrong field. Falls back to the first STRING option when none is named "prompt".
func (in discordInteraction) text() string {
	if in.Data == nil {
		return ""
	}
	var fallback string
	for _, o := range in.Data.Options {
		if o.Type != optionTypeString {
			continue
		}
		var s string
		if err := json.Unmarshal(o.Value, &s); err != nil || s == "" {
			continue
		}
		if o.Name == "prompt" {
			return s
		}
		if fallback == "" {
			fallback = s
		}
	}
	return fallback
}

