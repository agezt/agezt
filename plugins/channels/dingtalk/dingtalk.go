// SPDX-License-Identifier: MIT

// Package dingtalk is a two-way DingTalk channel over the enterprise robot
// "outgoing" model. When the robot is @-mentioned, DingTalk POSTs the message to
// this channel's Addr+Path (signed with timestamp+sign headers, verified against
// the robot's secret). Each inbound carries a short-lived `sessionWebhook` URL we
// POST the reply back to — so replies need no token fetch. Proactive briefs /
// `agt send` use the configured custom-robot webhook URL.
//
// An empty allowlist is fail-closed. Without an Addr the channel is send-only.
package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// DefaultPath is the inbound webhook route DingTalk should POST to.
	DefaultPath   = "/dingtalk"
	maxBody       = 1 << 20
	dtMaxChars    = 4000
	dedupCapacity = 2048
)

// Config configures the DingTalk channel.
type Config struct {
	WebhookURL string // custom-robot webhook URL for outbound briefs / agt send
	Secret     string // robot secret; verifies inbound timestamp+sign
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path       string // inbound route (default /dingtalk)
	HTTPClient *http.Client
}

// Channel is the DingTalk surface.
type Channel struct {
	cfg    Config
	path   string
	client *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a DingTalk channel, applying defaults.
func New(cfg Config) *Channel {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
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
func (c *Channel) Name() string { return "dingtalk" }

// Start serves the inbound webhook when an Addr is set; otherwise blocks.
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

// inbound is one normalized inbound DingTalk message.
type inbound struct {
	sender   string // senderStaffId (fallback senderNick), the allowlist key
	text     string
	id       string // msgId for dedup
	replyURL string // sessionWebhook to reply to
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
	if c.cfg.Secret != "" && !validSign(c.cfg.Secret, r.Header.Get("timestamp"), r.Header.Get("sign")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	if m, ok := parseInbound(body); ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "dingtalk",
		ChannelID:    m.sender,
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
	// Reply to the per-message sessionWebhook when present; else the robot webhook.
	url := m.replyURL
	if url == "" {
		url = c.cfg.WebhookURL
	}
	for _, chunk := range channel.SplitText(reply, dtMaxChars) {
		_ = c.post(ctx, url, chunk)
	}
}

// Send posts out.Text to the custom-robot webhook (proactive briefs / agt send).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	if c.cfg.WebhookURL == "" {
		return fmt.Errorf("dingtalk: robot webhook URL not configured")
	}
	for _, chunk := range channel.SplitText(text, dtMaxChars) {
		if err := c.post(ctx, c.cfg.WebhookURL, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.dingtalk", Kind: event.KindChannelOutbound, Actor: "channel-dingtalk",
			Payload: map[string]any{"channel_kind": "dingtalk", "channel_id": out.ChannelID, "text": text},
		})
	}
	return nil
}

func (c *Channel) post(ctx context.Context, url, text string) error {
	if url == "" {
		return fmt.Errorf("dingtalk: no reply URL")
	}
	payload := map[string]any{"msgtype": "text", "text": map[string]any{"content": text}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
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
		return fmt.Errorf("dingtalk: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.dingtalk",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-dingtalk",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "dingtalk", "channel_id": msg.ChannelID,
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

// validSign verifies DingTalk's outgoing signature:
// sign = base64(HMAC-SHA256(secret, timestamp + "\n" + secret)).
func validSign(secret, timestamp, sign string) bool {
	if timestamp == "" || sign == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(sign))) == 1
}

// parseInbound reads a DingTalk robot message: {msgtype, text:{content},
// senderStaffId, senderNick, msgId, sessionWebhook}.
func parseInbound(body []byte) (inbound, bool) {
	var d struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
		SenderStaffID  string `json:"senderStaffId"`
		SenderNick     string `json:"senderNick"`
		MsgID          string `json:"msgId"`
		SessionWebhook string `json:"sessionWebhook"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return inbound{}, false
	}
	if d.MsgType != "" && d.MsgType != "text" {
		return inbound{}, false
	}
	sender := d.SenderStaffID
	if sender == "" {
		sender = d.SenderNick
	}
	return inbound{
		sender:   sender,
		text:     strings.TrimSpace(d.Text.Content),
		id:       d.MsgID,
		replyURL: d.SessionWebhook,
	}, true
}
