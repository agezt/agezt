// SPDX-License-Identifier: MIT

// Package whatsappgw is a two-way WhatsApp channel over a self-hosted HTTP
// gateway — WAHA (https://waha.devlike.pro) or Evolution API
// (https://github.com/EvolutionAPI/evolution-api). Both run in Docker, log in by
// scanning a QR code (like WhatsApp Web), and expose a simple REST API — so this
// is the EASY WhatsApp path: no Meta Business account, no app review, no Cloud
// API. It's the same shape as the Signal channel (a local REST gateway), just
// with an inbound webhook instead of a long-poll.
//
// Outbound: POST a "send text" call (backend-specific URL/body/auth header).
// Inbound: the gateway POSTs a webhook to AGEZT (configure its webhook URL to
// point at this channel's Addr+Path); messages from allowlisted senders drive
// the agent and the reply is sent back as a fresh message. An empty allowlist is
// fail-closed (outbound-only). Inbound is optional — without an Addr the channel
// is send-only (notifications, briefs, `agt send`).
package whatsappgw

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
	// BackendWAHA / BackendEvolution select the gateway's REST dialect.
	BackendWAHA      = "waha"
	BackendEvolution = "evolution"
	// DefaultPath is the inbound webhook route the gateway should POST to.
	DefaultPath = "/whatsappgw"
	// DefaultSession is the WAHA session / Evolution instance name.
	DefaultSession = "default"
	maxBody        = 1 << 20
	waMaxChars     = 4000
	dedupCapacity  = 2048
)

// Config configures the gateway WhatsApp channel.
type Config struct {
	Backend    string // "waha" (default) or "evolution"
	BaseURL    string // gateway base URL, e.g. http://localhost:3000
	Session    string // WAHA session / Evolution instance (default "default")
	APIKey     string // gateway API key (WAHA X-Api-Key / Evolution apikey)
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path       string // inbound route (default /whatsappgw)
	Secret     string // optional shared secret; if set, inbound must echo it (X-Webhook-Secret)
	HTTPClient *http.Client
}

// Channel is the gateway WhatsApp surface.
type Channel struct {
	cfg     Config
	base    string
	session string
	path    string
	client  *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a gateway channel, applying defaults.
func New(cfg Config) *Channel {
	if cfg.Backend == "" {
		cfg.Backend = BackendWAHA
	}
	if cfg.Session == "" {
		cfg.Session = DefaultSession
	}
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		cfg:     cfg,
		base:    strings.TrimRight(cfg.BaseURL, "/"),
		session: cfg.Session,
		path:    cfg.Path,
		client:  client,
		seen:    make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "whatsappgw" }

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

// inbound is one normalized inbound message extracted from a gateway webhook.
type inbound struct {
	from string // bare WhatsApp number (jid suffix stripped)
	text string
	id   string // message id for dedup (best-effort)
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
	// Acknowledge promptly; process after (gateways may retry on non-2xx).
	w.WriteHeader(http.StatusOK)

	var msgs []inbound
	if c.cfg.Backend == BackendEvolution {
		msgs = parseEvolution(body)
	} else {
		msgs = parseWAHA(body)
	}
	for _, m := range msgs {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.from == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "whatsappgw",
		ChannelID:    m.from,
		Sender:       m.from,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.from)
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
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.from, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to out.ChannelID (a number or jid) via the gateway.
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
func parseWAHA(body []byte) []inbound {
	var w struct {
		Event   string `json:"event"`
		Payload struct {
			From   string `json:"from"`
			Body   string `json:"body"`
			ID     string `json:"id"`
			FromMe bool   `json:"fromMe"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil
	}
	if w.Event != "" && w.Event != "message" && w.Event != "message.any" {
		return nil
	}
	if w.Payload.FromMe || w.Payload.From == "" {
		return nil
	}
	return []inbound{{from: bareNumber(w.Payload.From), text: w.Payload.Body, id: w.Payload.ID}}
}

// parseEvolution reads an Evolution "messages.upsert" webhook:
// {event, data:{key:{remoteJid, id, fromMe}, message:{conversation | extendedTextMessage.text}}}.
func parseEvolution(body []byte) []inbound {
	var e struct {
		Event string `json:"event"`
		Data  struct {
			Key struct {
				RemoteJid string `json:"remoteJid"`
				ID        string `json:"id"`
				FromMe    bool   `json:"fromMe"`
			} `json:"key"`
			Message struct {
				Conversation        string `json:"conversation"`
				ExtendedTextMessage struct {
					Text string `json:"text"`
				} `json:"extendedTextMessage"`
			} `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return nil
	}
	if e.Data.Key.FromMe || e.Data.Key.RemoteJid == "" {
		return nil
	}
	text := e.Data.Message.Conversation
	if text == "" {
		text = e.Data.Message.ExtendedTextMessage.Text
	}
	return []inbound{{from: bareNumber(e.Data.Key.RemoteJid), text: text, id: e.Data.Key.ID}}
}

// bareNumber strips a WhatsApp jid suffix ("@c.us", "@s.whatsapp.net") to the
// bare number, the stable allowlist + reply key.
func bareNumber(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

// wahaChatID ensures a WAHA chatId has the "@c.us" suffix.
func wahaChatID(target string) string {
	if strings.Contains(target, "@") {
		return target
	}
	return target + "@c.us"
}
