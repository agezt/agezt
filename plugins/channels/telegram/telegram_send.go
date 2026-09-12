// SPDX-License-Identifier: MIT

// Telegram channel: outbound attachment send + media-type helper.
// Code extracted from telegram.go during the Day-95 god-file split.
// Public API unchanged.
package telegram


import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) sendAttachment(ctx context.Context, out channel.Outbound, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	method, field := "sendDocument", "document"
	switch att.Kind {
	case "audio":
		if strings.Contains(att.MIME, "ogg") || strings.Contains(att.MIME, "opus") {
			method, field = "sendVoice", "voice"
		} else {
			method, field = "sendAudio", "audio"
		}
	case "image":
		method, field = "sendPhoto", "photo"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", out.ChannelID)
	if out.ThreadID != "" {
		_ = mw.WriteField("message_thread_id", out.ThreadID)
	}
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	fw, err := mw.CreateFormFile(field, fn)
	if err != nil {
		return err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.client.Do(req)
	if err != nil {
		return c.scrubToken(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram %s: status %d", method, resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	payload := map[string]any{
		"channel_kind": msg.ChannelKind,
		"channel_id":   msg.ChannelID,
		"sender":       msg.Sender,
		"text":         msg.Text,
		"allowed":      allowed,
	}
	if msg.ThreadID != "" {
		payload["thread_id"] = msg.ThreadID // M885: history folds per topic
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.telegram",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-telegram",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	payload := map[string]any{
		"channel_kind": "telegram",
		"channel_id":   out.ChannelID,
		"text":         out.Text,
		"priority":     string(out.Priority),
	}
	if out.ThreadID != "" {
		payload["thread_id"] = out.ThreadID // M885
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.telegram",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-telegram",
		CorrelationID: corr,
		Payload:       payload,
	})
}
