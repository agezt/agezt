// SPDX-License-Identifier: MIT

// Package chatwebhook: Config + Channel + New + Name + Start + Handler +
// inbound struct + handleInbound + verify + dispatch + Send + sendOne +
// emitInbound + seenBefore. The platform-specific inbound payload parsers
// (parseInbound + parseMattermost + parseGoogleChat) moved to
// chatwebhook_parsers.go. Day-211 god-file split. Public API unchanged.
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
// endpoint URL. An empty configured token fails closed (no unauthenticated
// inbound) — the listener must never accept anonymous requests just because the
// operator set an ADDR and skipped the token.
func (c *Channel) verify(r *http.Request, body []byte) bool {
	if c.cfg.Token == "" {
		return false
	}
	var got string
	if c.cfg.Kind == KindMattermost {
		if vals, err := url.ParseQuery(string(body)); err == nil {
			got = vals.Get("token")
		}
	} else {
		got = r.URL.Query().Get("token")
	}
	if got == "" {
		return false
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
	rep, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
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
