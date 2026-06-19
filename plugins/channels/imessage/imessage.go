// SPDX-License-Identifier: MIT

// Package imessage is a two-way iMessage channel over a self-hosted BlueBubbles
// server (https://bluebubbles.app) running on a Mac. BlueBubbles exposes a REST
// API for sending and POSTs a webhook for each incoming message — exactly the
// same shape as the WhatsApp gateway (a local REST gateway with an inbound
// webhook), so this reuses that proven model.
//
// Outbound: POST /api/v1/message/text?password=… with {chatGuid, tempGuid,
// message, method}. Inbound: point the BlueBubbles server's webhook at this
// channel's Addr+Path; "new-message" events from allowlisted senders drive the
// agent and the reply is sent back into the same chat. An empty allowlist is
// fail-closed (outbound-only). Without an Addr the channel is send-only
// (notifications, briefs, `agt send`).
package imessage

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
	// DefaultPath is the inbound webhook route the BlueBubbles server should POST to.
	DefaultPath = "/imessage"
	// DefaultMethod is BlueBubbles' send method; "private-api" is the modern path,
	// "apple-script" the legacy fallback.
	DefaultMethod = "private-api"
	maxBody       = 1 << 20
	imMaxChars    = 10000
	dedupCapacity = 2048
)

// Config configures the iMessage (BlueBubbles) channel.
type Config struct {
	BaseURL    string // BlueBubbles server URL, e.g. http://localhost:1234
	Password   string // BlueBubbles server password (sent as ?password=)
	Method     string // "private-api" (default) or "apple-script"
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path       string // inbound route (default /imessage)
	Secret     string // optional shared secret; if set, inbound must echo it (X-Webhook-Secret)
	HTTPClient *http.Client
}

// Channel is the iMessage surface.
type Channel struct {
	cfg    Config
	base   string
	method string
	path   string
	client *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs an iMessage channel, applying defaults.
func New(cfg Config) *Channel {
	if cfg.Method == "" {
		cfg.Method = DefaultMethod
	}
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		cfg:    cfg,
		base:   strings.TrimRight(cfg.BaseURL, "/"),
		method: cfg.Method,
		path:   cfg.Path,
		client: client,
		seen:   make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "imessage" }

// Start serves the inbound webhook when an Addr is set; otherwise it blocks until
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

// inbound is one normalized inbound message extracted from a BlueBubbles webhook.
type inbound struct {
	chatGUID string // reply target (chats[0].guid)
	sender   string // handle address, for the allowlist
	text     string
	id       string // message guid for dedup
}

func (c *Channel) handleInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c.cfg.Secret != "" {
		got := r.Header.Get("X-Webhook-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(c.cfg.Secret)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	// Acknowledge promptly; process after (the server may retry on non-2xx).
	w.WriteHeader(http.StatusOK)
	if m, ok := parseWebhook(body); ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	target := m.chatGUID
	if target == "" {
		target = m.sender
	}
	if target == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	key := m.sender
	if key == "" {
		key = target
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "imessage",
		ChannelID:    target,
		Sender:       key,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(key)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.cfg.Handler == nil {
		return
	}
	rep, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" && len(rep.Attachments) == 0 {
		return
	}
	_ = c.Send(ctx, channel.Outbound{ChannelID: target, Text: reply, Attachments: rep.Attachments, Priority: channel.PriorityNotify})
}

// Send posts out.Text to out.ChannelID (a chat guid, phone number, or email) via
// the BlueBubbles server.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("imessage: send requires a chat guid or address")
	}
	if text == "" && len(out.Attachments) == 0 {
		return nil
	}
	if c.base == "" {
		return fmt.Errorf("imessage: BlueBubbles server URL not configured")
	}
	for _, chunk := range channel.SplitText(text, imMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		if err := c.sendOne(ctx, chatGUID(target), chunk); err != nil {
			return err
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendAttachment(ctx, chatGUID(target), att); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.imessage", Kind: event.KindChannelOutbound, Actor: "channel-imessage",
			Payload: map[string]any{"channel_kind": "imessage", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, guid, text string) error {
	endpoint := c.base + "/api/v1/message/text"
	if c.cfg.Password != "" {
		endpoint += "?password=" + url.QueryEscape(c.cfg.Password)
	}
	payload := map[string]any{
		"chatGuid": guid,
		"tempGuid": "agezt-" + ulid.New(),
		"message":  text,
		"method":   c.method,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
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
		return fmt.Errorf("imessage: BlueBubbles returned status %d", resp.StatusCode)
	}
	return nil
}

// sendAttachment uploads one media attachment to a chat via BlueBubbles'
// /api/v1/message/attachment endpoint (multipart/form-data).
func (c *Channel) sendAttachment(ctx context.Context, guid string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "attachment"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chatGuid", guid)
	_ = mw.WriteField("tempGuid", "agezt-"+ulid.New())
	_ = mw.WriteField("name", fn)
	_ = mw.WriteField("method", "private-api")
	fw, err := mw.CreateFormFile("attachment", fn)
	if err != nil {
		return err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	endpoint := c.base + "/api/v1/message/attachment"
	if c.cfg.Password != "" {
		endpoint += "?password=" + url.QueryEscape(c.cfg.Password)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("imessage: attachment upload returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.imessage",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-imessage",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "imessage", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

// seenBefore reports whether a message guid was already processed (replay guard),
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

// parseWebhook reads a BlueBubbles "new-message" webhook:
// {type, data:{guid, text, isFromMe, handle:{address}, chats:[{guid}]}}.
func parseWebhook(body []byte) (inbound, bool) {
	var w struct {
		Type string `json:"type"`
		Data struct {
			GUID     string `json:"guid"`
			Text     string `json:"text"`
			IsFromMe bool   `json:"isFromMe"`
			Handle   struct {
				Address string `json:"address"`
			} `json:"handle"`
			Chats []struct {
				GUID string `json:"guid"`
			} `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return inbound{}, false
	}
	if w.Type != "" && w.Type != "new-message" {
		return inbound{}, false
	}
	if w.Data.IsFromMe {
		return inbound{}, false
	}
	chat := ""
	if len(w.Data.Chats) > 0 {
		chat = w.Data.Chats[0].GUID
	}
	return inbound{
		chatGUID: chat,
		sender:   w.Data.Handle.Address,
		text:     w.Data.Text,
		id:       w.Data.GUID,
	}, true
}

// chatGUID normalizes a send target: a full BlueBubbles chat guid (contains ';')
// is used verbatim; a bare phone/email is wrapped as a direct iMessage guid.
func chatGUID(target string) string {
	if strings.Contains(target, ";") {
		return target
	}
	return "iMessage;-;" + target
}
