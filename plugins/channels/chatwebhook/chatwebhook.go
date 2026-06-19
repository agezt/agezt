// SPDX-License-Identifier: MIT

// Package chatwebhook is a two-way channel for chat platforms whose integration
// model is "incoming webhook out + a webhook POST in": Google Chat and
// Mattermost. Outbound (and proactive Pulse briefs) post to the configured
// incoming-webhook URL; inbound arrives as a webhook the platform POSTs to this
// channel's Addr+Path. Messages from allowlisted senders drive the agent and the
// reply is posted back via the same outbound webhook (async ack-then-reply, so a
// long agent run never times the inbound request out).
//
// This supersedes the outbound-only Google Chat / Mattermost entries in the push
// family when an inbound Addr is configured. An empty allowlist is fail-closed.
package chatwebhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// KindGoogleChat / KindMattermost select the inbound/outbound dialect.
	KindGoogleChat = "googlechat"
	KindMattermost = "mattermost"
	maxBody        = 1 << 20
	chatMaxChars   = 4000
	dedupCapacity  = 2048
)

// Config configures a chat-webhook channel.
type Config struct {
	Kind       string // "googlechat" or "mattermost"
	WebhookURL string // incoming-webhook URL for outbound + replies
	Token      string // verifies inbound (Mattermost outgoing-webhook token / Google Chat ?token=)
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path       string // inbound route (default /<kind>)
	HTTPClient *http.Client
}

// Channel is the chat-webhook surface.
type Channel struct {
	cfg    Config
	path   string
	client *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a chat-webhook channel, applying defaults.
func New(cfg Config) *Channel {
	cfg.Kind = strings.TrimSpace(strings.ToLower(cfg.Kind))
	if cfg.Path == "" {
		cfg.Path = "/" + cfg.Kind
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		cfg:    cfg,
		path:   cfg.Path,
		client: client,
		seen:   make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return c.cfg.Kind }

// Start serves the inbound webhook when an Addr is set; otherwise blocks until
// ctx is cancelled (outbound-only).
func (c *Channel) Start(ctx context.Context) error {
	if c.cfg.Addr == "" {
		<-ctx.Done()
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	srv := &http.Server{
		Addr:              c.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler exposes the inbound webhook handler (for embedding in a shared mux).
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

// inbound is one normalized inbound message.
type inbound struct {
	sender string // allowlist key (username / email / display name)
	target string // reply target (channel name / space); blank = the webhook's default
	text   string
	id     string // dedup key
}

func (c *Channel) handleInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !c.verify(r, body) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// Acknowledge promptly; run + reply asynchronously so a long agent run never
	// times the inbound webhook out.
	w.WriteHeader(http.StatusOK)
	m, ok := parseInbound(c.cfg.Kind, body)
	if ok {
		c.dispatch(r.Context(), m)
	}
}

// verify checks the inbound is authentic per platform. Mattermost outgoing
// webhooks carry a `token` form field; Google Chat callers append ?token= to the
// endpoint URL. If no Token is configured the check is skipped.
func (c *Channel) verify(r *http.Request, body []byte) bool {
	if c.cfg.Token == "" {
		return true
	}
	var got string
	if c.cfg.Kind == KindMattermost {
		if vals, err := url.ParseQuery(string(body)); err == nil {
			got = vals.Get("token")
		}
	} else {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(c.cfg.Token)) == 1
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  c.cfg.Kind,
		ChannelID:    m.target,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.sender)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.cfg.Handler == nil {
		return
	}
	reply, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply == "" {
		return
	}
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.target, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to the incoming webhook. For Mattermost, out.ChannelID (a
// channel name) overrides the webhook's default channel; for Google Chat the
// space is fixed by the webhook URL.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	if c.cfg.WebhookURL == "" {
		return fmt.Errorf("%s: webhook URL not configured", c.cfg.Kind)
	}
	for _, chunk := range channel.SplitText(text, chatMaxChars) {
		if err := c.sendOne(ctx, strings.TrimSpace(out.ChannelID), chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound." + c.cfg.Kind, Kind: event.KindChannelOutbound, Actor: "channel-" + c.cfg.Kind,
			Payload: map[string]any{"channel_kind": c.cfg.Kind, "channel_id": out.ChannelID, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, target, text string) error {
	payload := map[string]any{"text": text}
	if c.cfg.Kind == KindMattermost && target != "" {
		payload["channel"] = target
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: webhook returned status %d", c.cfg.Kind, resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound." + c.cfg.Kind,
		Kind:          event.KindChannelInbound,
		Actor:         "channel-" + c.cfg.Kind,
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": c.cfg.Kind, "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

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
