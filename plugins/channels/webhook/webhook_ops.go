// SPDX-License-Identifier: MIT

package webhook

// Webhook channel OUTBOUND ops: verify + Send + send +
// emitInbound + emitOutbound. Carved out of webhook.go during the
// Day 192 god-file split so the main file can stay focused on
// types + lifecycle + inbound handleInbound and the helpers file
// can stay focused on signing + JSON + dedup helpers.
// Public API unchanged.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) verify(sig string, body []byte) bool {
	if c.secret == "" || sig == "" {
		return false
	}
	got := strings.TrimPrefix(sig, "sha256=")
	want := sign(c.secret, body)
	return hmac.Equal([]byte(got), []byte(want))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: POST the message to the configured
// OutboundURL, signed with the same scheme inbound expects. Errors when no
// OutboundURL is configured (the channel is inbound/synchronous-reply only).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if c.outboundURL == "" {
		return fmt.Errorf("webhook: no outbound URL configured (set it to send async messages)")
	}
	corr := "chan-" + ulid.New()
	return c.send(ctx, out, corr)
}

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	body, err := json.Marshal(map[string]any{
		"channel_id": out.ChannelID,
		"text":       out.Text,
		"priority":   string(out.Priority),
		"ts_ms":      c.now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.outboundURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Agezt-Signature", "sha256="+sign(c.secret, body))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("webhook: outbound POST returned status %d", resp.StatusCode)
	}
	c.emitOutbound(out, corr)
	return nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.webhook",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-webhook",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": msg.ChannelKind,
			"channel_id":   msg.ChannelID,
			"sender":       msg.Sender,
			"text":         msg.Text,
			"allowed":      allowed,
		},
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.webhook",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-webhook",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// sign returns the hex HMAC-SHA256 of body under secret (same as the outbound
// webhook dispatcher in kernel/webhook).
