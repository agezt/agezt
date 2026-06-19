// SPDX-License-Identifier: MIT

// Package line is a two-way LINE channel over the official LINE Messaging API.
// Inbound: LINE POSTs a webhook (signed with X-Line-Signature = base64(HMAC-
// SHA256(channelSecret, body))); text messages from allowlisted users drive the
// agent and the reply goes back via the (free) reply-token endpoint. Outbound /
// proactive briefs use the push endpoint. An empty allowlist is fail-closed
// (outbound-only). Without an Addr the channel is send-only.
//
// This supersedes the outbound-only LINE entry in the push family when a channel
// secret + inbound Addr are configured (AGEZT_LINE_SECRET + AGEZT_LINE_ADDR).
package line

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
	// DefaultPath is the inbound webhook route LINE should POST to.
	DefaultPath    = "/line"
	defaultAPIBase = "https://api.line.me"
	maxBody        = 1 << 20
	lineMaxChars   = 5000
	dedupCapacity  = 2048
)

// Config configures the LINE channel.
type Config struct {
	Secret      string // channel secret (verifies inbound X-Line-Signature)
	AccessToken string // channel access token (Bearer for reply/push)
	Allowlist   channel.Allowlist
	Bus         *bus.Bus
	Handler     channel.InboundHandler
	Addr        string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path        string // inbound route (default /line)
	APIBase     string // LINE API base (default https://api.line.me); overridable for tests
	HTTPClient  *http.Client
}

// Channel is the LINE surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	client  *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a LINE channel, applying defaults.
func New(cfg Config) *Channel {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		cfg:     cfg,
		path:    cfg.Path,
		apiBase: base,
		client:  client,
		seen:    make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "line" }

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

// inbound is one normalized inbound LINE message.
type inbound struct {
	userID     string
	replyToken string
	text       string
	id         string
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
	if c.cfg.Secret != "" && !validSignature(c.cfg.Secret, body, r.Header.Get("X-Line-Signature")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	for _, m := range parseWebhook(body) {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.userID == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "line",
		ChannelID:    m.userID,
		Sender:       m.userID,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.userID)
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
	// Prefer the free reply-token endpoint; fall back to push if it's missing.
	if m.replyToken != "" {
		_ = c.send(ctx, c.apiBase+"/v2/bot/message/reply", map[string]any{"replyToken": m.replyToken, "messages": textMessages(reply)})
	} else {
		_ = c.Send(ctx, channel.Outbound{ChannelID: m.userID, Text: reply, Priority: channel.PriorityNotify})
	}
}

// Send pushes out.Text to out.ChannelID (a LINE userId/groupId) via the push API.
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

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.line",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-line",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "line", "channel_id": msg.ChannelID,
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

// textMessages splits text into LINE message objects (max 5 per request, each
// ≤5000 chars).
func textMessages(text string) []map[string]any {
	chunks := channel.SplitText(text, lineMaxChars)
	if len(chunks) > 5 {
		chunks = chunks[:5]
	}
	msgs := make([]map[string]any, 0, len(chunks))
	for _, ch := range chunks {
		msgs = append(msgs, map[string]any{"type": "text", "text": ch})
	}
	return msgs
}

// validSignature checks X-Line-Signature = base64(HMAC-SHA256(secret, body)).
func validSignature(secret string, body []byte, header string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(header))) == 1
}

// parseWebhook reads a LINE webhook: {events:[{type, replyToken, source:{userId},
// message:{type, id, text}}]}. Only text messages from users are kept.
func parseWebhook(body []byte) []inbound {
	var w struct {
		Events []struct {
			Type       string `json:"type"`
			ReplyToken string `json:"replyToken"`
			Source     struct {
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
			} `json:"source"`
			Message struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil
	}
	var out []inbound
	for _, e := range w.Events {
		if e.Type != "message" || e.Message.Type != "text" {
			continue
		}
		from := e.Source.UserID
		if from == "" {
			from = e.Source.GroupID
		}
		out = append(out, inbound{userID: from, replyToken: e.ReplyToken, text: e.Message.Text, id: e.Message.ID})
	}
	return out
}
