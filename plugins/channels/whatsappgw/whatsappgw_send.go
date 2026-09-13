// SPDX-License-Identifier: MIT

package whatsappgw

// WhatsApp outbound Send + emit helpers: Send + sendOne +
// emitInbound + seenBefore + bareNumber + wahaChatID. Carved out
// of whatsappgw.go during the Day 196 god-file split so the main
// file can stay focused on types + lifecycle + inbound handling
// and the parse file can stay focused on gateway-specific JSON
// parsers.
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
		return fmt.Errorf("whatsappgw: send requires a target number")
	}
	if text == "" {
		return nil
	}
	if c.base == "" {
		return fmt.Errorf("whatsappgw: gateway URL not configured")
	}
	for _, chunk := range channel.SplitText(text, waMaxChars) {
		if err := c.sendOne(ctx, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.whatsappgw", Kind: event.KindChannelOutbound, Actor: "channel-whatsappgw",
			Payload: map[string]any{"channel_kind": "whatsappgw", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, target, text string) error {
	var url string
	var payload map[string]any
	var keyHeader string
	if c.cfg.Backend == BackendEvolution {
		// Evolution: POST /message/sendText/{instance}, {number, text}, apikey header.
		url = fmt.Sprintf("%s/message/sendText/%s", c.base, c.session)
		payload = map[string]any{"number": bareNumber(target), "text": text}
		keyHeader = "apikey"
	} else {
		// WAHA: POST /api/sendText, {session, chatId, text}, X-Api-Key header.
		url = c.base + "/api/sendText"
		payload = map[string]any{"session": c.session, "chatId": wahaChatID(target), "text": text}
		keyHeader = "X-Api-Key"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set(keyHeader, c.cfg.APIKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("whatsappgw: gateway returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.whatsappgw",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-whatsappgw",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "whatsappgw", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

// seenBefore reports whether a message id was already processed (replay guard),
// recording it otherwise. Bounded by a small ring.
func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCapacity {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}

// ---- wire shapes ---------------------------------------------------------

// parseWAHA reads a WAHA "message" webhook: {event, payload:{from, body, id, fromMe}}.
