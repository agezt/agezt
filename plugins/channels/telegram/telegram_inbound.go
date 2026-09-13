// SPDX-License-Identifier: MIT

package telegram

// Telegram inbound receive flow: getUpdates + handleInbound. Carved
// out of telegram.go during the Day 188 god-file split so the main
// file can stay focused on Channel/Config types + lifecycle + the
// outbound file can stay focused on Send + media fetch.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	q := url.Values{}
	q.Set("timeout", strconv.Itoa(c.pollSecs))
	if c.offset > 0 {
		q.Set("offset", strconv.FormatInt(c.offset, 10))
	}
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?%s", c.base, c.token, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, c.scrubToken(err)
	}
	defer resp.Body.Close()
	var out getUpdatesResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, tgAPIMaxResponseBytes)).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates: not ok")
	}
	return out.Result, nil
}

// handleInbound normalizes one message, enforces the allowlist, runs the
// handler, and replies. All steps journaled so `agt why`/`agt inbox` can
// reconstruct the exchange.
func (c *Channel) handleInbound(ctx context.Context, m *tgMessage) {
	chatID := strconv.FormatInt(m.Chat.ID, 10)
	sender := chatID
	if m.From != nil && m.From.Username != "" {
		sender = m.From.Username
	}
	// A photo carries its text as a caption, not in Text.
	text := m.Text
	if text == "" {
		text = m.Caption
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "telegram",
		ChannelID:    chatID,
		Sender:       sender,
		Text:         text,
		PlatformTSMS: m.Date * 1000,
		PlatformMeta: map[string]string{"message_id": strconv.FormatInt(m.MessageID, 10)},
	}
	// Forum topic = its own conversation thread (M885); the reply goes back
	// into the topic and history folds per topic.
	if m.IsTopicMessage && m.MessageThreadID != 0 {
		msg.ThreadID = strconv.FormatInt(m.MessageThreadID, 10)
	}

	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(chatID)
	// Inbound photo (M247): fetch the largest size as a data: URL so a vision
	// model can see it. Only for allowlisted senders — never dereference a
	// file reference from an unauthorized sender.
	if allowed && len(m.Photo) > 0 {
		largest := m.Photo[len(m.Photo)-1]
		if du, err := c.fetchPhotoDataURL(ctx, largest.FileID); err == nil && du != "" {
			msg.Images = []string{du}
		}
	}
	// Inbound voice note: fetch the OGG/Opus as a data: URL so the daemon can
	// transcribe it (auto-STT). Same allowlist gate as photos.
	if allowed && m.Voice != nil && m.Voice.FileID != "" {
		if du, err := c.fetchPhotoDataURL(ctx, m.Voice.FileID); err == nil && du != "" {
			msg.Audio = []string{du}
		}
	}
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		// Fail-closed: a non-allowlisted sender cannot drive the agent.
		// Tell them once so it isn't a silent black hole.
		_ = c.send(ctx, channel.Outbound{ChannelID: chatID, Text: "not authorized"}, "")
		return
	}
	if c.handler == nil {
		return
	}

	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" && len(rep.Attachments) == 0 {
		return
	}
	_ = c.send(ctx, channel.Outbound{ChannelID: chatID, ThreadID: msg.ThreadID, Text: reply, Attachments: rep.Attachments, Priority: channel.PriorityNotify}, corr)
}

// tgPhotoMaxRaw bounds a downloaded photo so the resulting data: URL stays
// within the control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
const tgPhotoMaxRaw = 12 << 20

// fetchPhotoDataURL resolves a Telegram photo file_id to an inline data: URL.
// Telegram needs two calls: getFile to learn the file_path, then a download
// from the /file/bot<token>/ endpoint. The bytes are read here in the channel
// (the daemon holds the bot token, not the provider) and handed onward as a
// self-describing data: URL the vision providers emit natively (M247).
