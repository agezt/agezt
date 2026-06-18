// SPDX-License-Identifier: MIT

// Package push is a family of outbound notification channels that share one
// shape — "POST this text to a service" — so AGEZT ships with many notification
// destinations out of the box without a bespoke package each. Each provider is
// its own channel.Channel (its Name() is the provider kind: "ntfy", "pushover",
// …), so it appears distinctly in the Channels wizard and routes via the live
// channels map and Pulse like any other channel.
//
// All providers here are OUTBOUND-ONLY (notifications + `agt send` + Pulse
// briefs); receiving from them needs a separate inbound model. Start blocks
// until ctx is cancelled to keep the per-channel lifecycle uniform. Secrets
// (tokens, webhook URLs) are never logged.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

// Known provider kinds.
const (
	KindNtfy       = "ntfy"
	KindPushover   = "pushover"
	KindGotify     = "gotify"
	KindPushbullet = "pushbullet"
	KindGoogleChat = "googlechat"
	KindMattermost = "mattermost"
)

// Config configures one push provider. Only the fields relevant to Kind are
// used; New validates that the required ones are present.
type Config struct {
	Kind   string
	Server string // ntfy/gotify base URL (ntfy defaults to https://ntfy.sh)
	Topic  string // ntfy topic
	Token  string // pushover/gotify app token, pushbullet access token, optional ntfy bearer
	User   string // pushover user/group key
	URL    string // googlechat/mattermost incoming webhook URL

	Bus        *bus.Bus
	HTTPClient *http.Client
}

// Channel is one outbound push provider.
type Channel struct {
	cfg    Config
	client *http.Client
}

// New builds a push Channel, returning an error if Kind is unknown or required
// fields for that Kind are missing.
func New(cfg Config) (*Channel, error) {
	cfg.Kind = strings.TrimSpace(strings.ToLower(cfg.Kind))
	missing := func(field string) error {
		return fmt.Errorf("push: %s requires %s", cfg.Kind, field)
	}
	switch cfg.Kind {
	case KindNtfy:
		if strings.TrimSpace(cfg.Server) == "" {
			cfg.Server = "https://ntfy.sh"
		}
		if strings.TrimSpace(cfg.Topic) == "" {
			return nil, missing("a topic")
		}
	case KindPushover:
		if cfg.Token == "" || cfg.User == "" {
			return nil, missing("an app token and user key")
		}
	case KindGotify:
		if strings.TrimSpace(cfg.Server) == "" || cfg.Token == "" {
			return nil, missing("a server URL and app token")
		}
	case KindPushbullet:
		if cfg.Token == "" {
			return nil, missing("an access token")
		}
	case KindGoogleChat, KindMattermost:
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, missing("a webhook URL")
		}
	default:
		return nil, fmt.Errorf("push: unknown provider %q", cfg.Kind)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{cfg: cfg, client: client}, nil
}

// Name implements channel.Channel (the provider kind).
func (c *Channel) Name() string { return c.cfg.Kind }

// Start implements channel.Channel. Push providers are outbound-only, so Start
// just blocks until ctx is cancelled.
func (c *Channel) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Send delivers out.Text to the provider. Empty text is a no-op.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	req, err := c.buildRequest(ctx, text)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("push: %s send: %w", c.cfg.Kind, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("push: %s send: %s: %s", c.cfg.Kind, resp.Status, strings.TrimSpace(string(body)))
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel." + c.cfg.Kind, Kind: event.KindChannelOutbound, Actor: c.cfg.Kind,
			Payload: map[string]any{"channel_kind": c.cfg.Kind, "channel_id": out.ChannelID, "text": text},
		})
	}
	return nil
}

// buildRequest constructs the provider-specific HTTP request for text.
func (c *Channel) buildRequest(ctx context.Context, text string) (*http.Request, error) {
	jsonReq := func(u string, body any) (*http.Request, error) {
		b, _ := json.Marshal(body)
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}
	switch c.cfg.Kind {
	case KindNtfy:
		u := strings.TrimRight(c.cfg.Server, "/") + "/" + url.PathEscape(c.cfg.Topic)
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(text))
		if err != nil {
			return nil, err
		}
		if c.cfg.Token != "" {
			r.Header.Set("Authorization", "Bearer "+c.cfg.Token)
		}
		return r, nil
	case KindPushover:
		form := url.Values{"token": {c.cfg.Token}, "user": {c.cfg.User}, "message": {text}}
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.pushover.net/1/messages.json", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r, nil
	case KindGotify:
		u := strings.TrimRight(c.cfg.Server, "/") + "/message?token=" + url.QueryEscape(c.cfg.Token)
		return jsonReq(u, map[string]any{"message": text})
	case KindPushbullet:
		r, err := jsonReq("https://api.pushbullet.com/v2/pushes", map[string]any{"type": "note", "body": text})
		if err != nil {
			return nil, err
		}
		r.Header.Set("Access-Token", c.cfg.Token)
		return r, nil
	case KindGoogleChat, KindMattermost:
		return jsonReq(c.cfg.URL, map[string]any{"text": text})
	default:
		return nil, fmt.Errorf("push: unknown provider %q", c.cfg.Kind)
	}
}
