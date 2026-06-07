// SPDX-License-Identifier: MIT

// Package teams is an outbound channel that delivers Agezt messages — Pulse
// briefs and `agt send` — to Microsoft Teams via Incoming Webhooks (SPEC-04 §1).
// A brief lands as a card in the target Teams channel.
//
// Teams Incoming Webhooks are per-channel URLs, so this channel holds a NAMED MAP
// of webhooks (e.g. "general" → https://…, "alerts" → https://…). The outbound
// message's "channel_id" selects which named webhook to post to; an unknown name
// is refused (fail-closed), so a misconfigured brief can't post to an
// unintended URL. Send POSTs a MessageCard (`{"@type":"MessageCard","text":…}`),
// the long-standing Incoming Webhook payload.
//
// It is outbound-only: receiving from Teams needs the Bot Framework (a separate
// auth model), out of scope here. Start blocks until ctx is cancelled to keep the
// daemon's per-channel lifecycle uniform.
//
// Security (SPEC-04 §1.7): outbound only; webhook URLs (which are bearer secrets)
// are never logged; an unknown channel name is refused.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// Config configures the outbound Teams channel.
type Config struct {
	// Webhooks maps a channel NAME to its Teams Incoming Webhook URL. The outbound
	// message's channel_id selects the name; an unknown name is refused.
	Webhooks map[string]string
	// Bus journals channel.outbound events. May be nil.
	Bus *bus.Bus
	// HTTPClient is used for the POST; nil → a 30s-timeout client.
	HTTPClient *http.Client
}

// Channel is the outbound Microsoft Teams messaging surface.
type Channel struct {
	webhooks map[string]string
	bus      *bus.Bus
	client   *http.Client
}

// New constructs a Teams channel from cfg.
func New(cfg Config) *Channel {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	hooks := make(map[string]string, len(cfg.Webhooks))
	for name, url := range cfg.Webhooks {
		name = strings.TrimSpace(name)
		url = strings.TrimSpace(url)
		if name != "" && url != "" {
			hooks[name] = url
		}
	}
	return &Channel{webhooks: hooks, bus: cfg.Bus, client: client}
}

// Names returns the configured webhook names (for Pulse fan-out + status).
func (c *Channel) Names() []string {
	out := make([]string, 0, len(c.webhooks))
	for name := range c.webhooks {
		out = append(out, name)
	}
	return out
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "teams" }

// Start implements channel.Channel. Teams is outbound-only, so Start just blocks
// until ctx is cancelled (keeping the per-channel lifecycle uniform).
func (c *Channel) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Send posts out.Text to the named Teams webhook (out.ChannelID). Empty text is a
// no-op; an unknown name is refused (fail-closed).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	name := strings.TrimSpace(out.ChannelID)
	if name == "" {
		return fmt.Errorf("teams: webhook name (channel_id) required")
	}
	url, ok := c.webhooks[name]
	if !ok {
		return fmt.Errorf("teams: no webhook configured named %q", name)
	}
	payload, err := json.Marshal(map[string]any{
		"@type":    "MessageCard",
		"@context": "http://schema.org/extensions",
		"text":     out.Text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("teams: send: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("teams: webhook returned status %d", resp.StatusCode)
	}
	c.emitOutbound(out)
	return nil
}

func (c *Channel) emitOutbound(out channel.Outbound) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.teams",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-teams",
		CorrelationID: "chan-" + ulid.New(),
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}
