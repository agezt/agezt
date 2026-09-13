// SPDX-License-Identifier: MIT

package line

// LINE outbound Send: Send + send. Carved out of line.go during the
// Day 191 god-file split so the main file can stay focused on
// Config/Channel types + lifecycle + inbound webhook handling and
// the helpers file can stay focused on emit/seen/text/fetch/sig/
// parse helpers.
// Public API unchanged.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("line: send requires a userId/groupId")
	}
	if text == "" {
		return nil
	}
	if err := c.send(ctx, c.apiBase+"/v2/bot/message/push", map[string]any{"to": target, "messages": textMessages(text)}); err != nil {
		return err
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.line", Kind: event.KindChannelOutbound, Actor: "channel-line",
			Payload: map[string]any{"channel_kind": "line", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) send(ctx context.Context, url string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("line: API returned status %d", resp.StatusCode)
	}
	return nil
}

