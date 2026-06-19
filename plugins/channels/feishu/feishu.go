// SPDX-License-Identifier: MIT

// Package feishu is a two-way Feishu / Lark channel over an app event
// subscription. Feishu POSTs events to this channel's Addr+Path: a one-time
// url_verification challenge (echoed back) and im.message.receive_v1 message
// events (verified against the app's verification token). Replies are sent via
// the IM API using a tenant_access_token fetched from the app id/secret (cached
// until expiry). An empty allowlist is fail-closed; without an Addr the channel
// is send-only (briefs to a configured chat).
package feishu

import (
	"bytes"
	"context"
	"crypto/subtle"
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
	// DefaultPath is the inbound event route Feishu should POST to.
	DefaultPath    = "/feishu"
	defaultAPIBase = "https://open.feishu.cn"
	maxBody        = 1 << 20
	feishuMaxChars = 4000
	dedupCapacity  = 2048
)

// Config configures the Feishu channel.
type Config struct {
	AppID       string // app id (fetches tenant_access_token)
	AppSecret   string // app secret
	VerifyToken string // event "token" (verifies inbound)
	DefaultChat string // chat_id for proactive briefs / agt send without a target
	Allowlist   channel.Allowlist
	Bus         *bus.Bus
	Handler     channel.InboundHandler
	Addr        string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path        string // inbound route (default /feishu)
	APIBase     string // Feishu API base (default https://open.feishu.cn); overridable for tests
	HTTPClient  *http.Client
}

// Channel is the Feishu surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	client  *http.Client

	tmu      sync.Mutex
	token    string
	tokenExp time.Time

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a Feishu channel, applying defaults.
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
func (c *Channel) Name() string { return "feishu" }

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

type inbound struct {
	sender string // open_id, the allowlist key
	chatID string // reply target
	text   string
	id     string // event/message id for dedup
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
	// url_verification handshake: echo the challenge.
	if challenge, tok, ok := urlVerification(body); ok {
		if c.cfg.VerifyToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(c.cfg.VerifyToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		return
	}
	m, tok, ok := parseEvent(body)
	if c.cfg.VerifyToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(c.cfg.VerifyToken)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	if ok {
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
		ChannelKind:  "feishu",
		ChannelID:    m.chatID,
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
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.chatID, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to a Feishu chat (out.ChannelID, else the default chat).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	if target == "" {
		target = c.cfg.DefaultChat
	}
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("feishu: send requires a chat_id")
	}
	if text == "" {
		return nil
	}
	tok, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	for _, chunk := range channel.SplitText(text, feishuMaxChars) {
		if err := c.sendOne(ctx, tok, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.feishu", Kind: event.KindChannelOutbound, Actor: "channel-feishu",
			Payload: map[string]any{"channel_kind": "feishu", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, token, chatID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]any{"receive_id": chatID, "msg_type": "text", "content": string(content)}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("feishu: send returned status %d", resp.StatusCode)
	}
	return nil
}

// tenantToken returns a cached tenant_access_token, fetching a fresh one when
// expired.
func (c *Channel) tenantToken(ctx context.Context) (string, error) {
	c.tmu.Lock()
	defer c.tmu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	payload, _ := json.Marshal(map[string]string{"app_id": c.cfg.AppID, "app_secret": c.cfg.AppSecret})
	url := c.apiBase + "/open-apis/auth/v3/tenant_access_token/internal"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var tr struct {
		Code  int    `json:"code"`
		Token string `json:"tenant_access_token"`
		Exp   int    `json:"expire"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.Token == "" {
		return "", fmt.Errorf("feishu: token fetch failed (code %d)", tr.Code)
	}
	c.token = tr.Token
	exp := tr.Exp
	if exp <= 0 {
		exp = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(exp-60) * time.Second)
	return c.token, nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.feishu",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-feishu",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "feishu", "channel_id": msg.ChannelID,
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

// urlVerification detects the one-time challenge POST and returns (challenge,
// token, true).
func urlVerification(body []byte) (string, string, bool) {
	var v struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", "", false
	}
	if v.Type != "url_verification" || v.Challenge == "" {
		return "", "", false
	}
	return v.Challenge, v.Token, true
}

// parseEvent reads an im.message.receive_v1 event (schema 2.0). The message
// content is itself a JSON string ({"text":"…"}). Returns (msg, token, ok).
func parseEvent(body []byte) (inbound, string, bool) {
	var e struct {
		Header struct {
			Token     string `json:"token"`
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
		} `json:"header"`
		Event struct {
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
			} `json:"sender"`
			Message struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"`
			} `json:"message"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return inbound{}, "", false
	}
	if e.Header.EventType != "im.message.receive_v1" || e.Event.Message.MessageType != "text" {
		return inbound{}, e.Header.Token, false
	}
	var content struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(e.Event.Message.Content), &content)
	id := e.Event.Message.MessageID
	if id == "" {
		id = e.Header.EventID
	}
	return inbound{
		sender: e.Event.Sender.SenderID.OpenID,
		chatID: e.Event.Message.ChatID,
		text:   strings.TrimSpace(content.Text),
		id:     id,
	}, e.Header.Token, true
}
