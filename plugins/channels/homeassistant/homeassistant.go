// SPDX-License-Identifier: MIT

// Package homeassistant is an outbound channel that delivers Agezt messages —
// Pulse briefs and `agt send` — to a Home Assistant instance via its REST
// notify API (SPEC-04 §1). It turns the agentic OS into a voice in your home:
// a brief lands as a phone push, a TTS announcement, or a persistent
// notification, depending on which notify service you target.
//
// The "channel_id" of an outbound message is the Home Assistant notify SERVICE
// name (e.g. "mobile_app_phone", "persistent_notification", "tts"); Send POSTs
// to POST {base}/api/services/notify/{service} with a long-lived access token.
// An Allowlist restricts which services Agezt may call (so a misconfigured brief
// can't trigger arbitrary services). Transport is stdlib net/http (no new
// dependency); the HTTP client is injectable so message construction is
// unit-testable without a live Home Assistant.
//
// It is outbound-only: driving an agent FROM Home Assistant is done by an HA
// automation POSTing to the generic webhook channel (kernel-signed), so there's
// no inbound injection surface here. Start blocks until ctx is cancelled to keep
// the daemon's per-channel lifecycle uniform.
//
// Security (SPEC-04 §1.7): outbound only; the access token is never logged; the
// Allowlist is fail-closed (empty → calls no service).
package homeassistant

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

// Config configures the outbound Home Assistant channel.
type Config struct {
	// BaseURL is the Home Assistant root (e.g. "http://homeassistant.local:8123").
	BaseURL string
	// Token is a long-lived access token (Settings → Profile → Long-Lived Tokens).
	Token string
	// Allowlist restricts which notify service names may be called (fail-closed).
	Allowlist channel.Allowlist
	// Bus journals channel.outbound events. May be nil.
	Bus *bus.Bus
	// HTTPClient is used for the REST call; nil → a 30s-timeout client.
	HTTPClient *http.Client
}

// Channel is the outbound Home Assistant messaging surface.
type Channel struct {
	baseURL string
	token   string
	allow   channel.Allowlist
	bus     *bus.Bus
	client  *http.Client
}

// New constructs a Home Assistant channel from cfg.
func New(cfg Config) *Channel {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		allow:   cfg.Allowlist,
		bus:     cfg.Bus,
		client:  client,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "homeassistant" }

// Start implements channel.Channel. Home Assistant is outbound-only, so Start
// just blocks until ctx is cancelled (keeping the per-channel lifecycle uniform).
func (c *Channel) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Send delivers out.Text to the Home Assistant notify service named by
// out.ChannelID. Empty text is a no-op; a non-allowlisted service is refused.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	service := strings.TrimSpace(out.ChannelID)
	if service == "" {
		return fmt.Errorf("homeassistant: notify service (channel_id) required")
	}
	if !c.allow.Allows(service) {
		return fmt.Errorf("homeassistant: service %q not in allowlist", service)
	}
	if c.baseURL == "" || c.token == "" {
		return fmt.Errorf("homeassistant: base URL and token required")
	}
	payload, err := json.Marshal(map[string]any{"message": out.Text})
	if err != nil {
		return err
	}
	endpoint := c.baseURL + "/api/services/notify/" + service
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("homeassistant: send: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("homeassistant: notify returned status %d", resp.StatusCode)
	}
	c.emitOutbound(out)
	return nil
}

func (c *Channel) emitOutbound(out channel.Outbound) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.homeassistant",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-homeassistant",
		CorrelationID: "chan-" + ulid.New(),
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}
