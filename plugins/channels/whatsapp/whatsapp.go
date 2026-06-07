// SPDX-License-Identifier: MIT

// Package whatsapp is an inbound/outbound WhatsApp channel over Meta's WhatsApp
// Cloud API (SPEC-04 §1). An allowlisted WhatsApp number can drive an Agezt agent
// by messaging the business number; the agent replies in the same thread.
// Proactive messages (Pulse briefs, `agt send`) go out via the Graph API.
//
// Inbound has two shapes Meta defines:
//   - GET verification handshake: Meta calls with hub.mode=subscribe,
//     hub.verify_token, hub.challenge — the handler echoes the challenge when the
//     token matches (one-time webhook setup).
//   - POST delivery: Meta posts a JSON envelope (entry[].changes[].value.messages[]).
//     The handler authenticates it with the X-Hub-Signature-256 header
//     (sha256=<hex HMAC-SHA256(appSecret, raw body)>); an empty app secret fails
//     closed, so no unsigned inbound.
//
// WhatsApp has no synchronous reply, so the agent's answer is sent back as a fresh
// Graph API message to the sender (the same path as Send); the webhook returns 200
// promptly. Outbound: POST /{PhoneNumberID}/messages with a Bearer access token;
// long text is split with channel.SplitText.
//
// Security (SPEC-04 §1.7): inbound text is data, never kernel instructions; the
// signature authenticates Meta; an Allowlist of sender numbers gates who may drive
// the agent (fail-closed); a dedup set on the message id guards Meta's retries;
// bodies are length-bounded. The agent's own tool calls still pass through Edict.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	// DefaultPath is the inbound route Meta calls (verification + deliveries).
	DefaultPath = "/whatsapp"
	// DefaultGraphBase is the Meta Graph API root for outbound sends.
	DefaultGraphBase = "https://graph.facebook.com/v21.0"
	// maxBody bounds an inbound request body.
	maxBody = 1 << 20
	// waMaxChars caps each outbound text message (WhatsApp allows 4096).
	waMaxChars = 4000
	// dedupCapacity bounds the replay-guard set of recently-seen message ids.
	dedupCapacity = 4096
)

// Config configures a WhatsApp Channel.
type Config struct {
	// Addr is the local address to serve the inbound route on, typically fronted
	// by a tunnel/reverse proxy. Empty → outbound-only.
	Addr string
	// Path is the inbound route; empty defaults to DefaultPath.
	Path string
	// VerifyToken is the secret echoed back during Meta's GET verification
	// handshake. Empty disables the handshake (returns 403).
	VerifyToken string
	// AppSecret signs inbound deliveries (X-Hub-Signature-256). Empty disables
	// inbound deliveries (fail closed — no unsigned commands).
	AppSecret string
	// AccessToken is the Bearer token for outbound Graph API sends.
	AccessToken string
	// PhoneNumberID is the WhatsApp business phone-number id outbound posts to.
	PhoneNumberID string
	// Allowlist gates which sender numbers may drive the agent.
	Allowlist channel.Allowlist
	// Bus journals channel.inbound/outbound events. May be nil.
	Bus *bus.Bus
	// Handler runs the agent for an inbound message. Required for inbound.
	Handler channel.InboundHandler
	// GraphBase overrides the Graph API root (tests point it at a mock). Empty →
	// DefaultGraphBase.
	GraphBase string
	// HTTPClient is used for outbound sends; nil → a 30s-timeout client.
	HTTPClient *http.Client
}

// Channel is the WhatsApp Cloud API messaging surface.
type Channel struct {
	addr        string
	path        string
	verifyToken string
	appSecret   string
	accessToken string
	phoneID     string
	allow       channel.Allowlist
	bus         *bus.Bus
	handler     channel.InboundHandler
	graphBase   string
	client      *http.Client
	dedup       *dedup
}

// New constructs a WhatsApp Channel from cfg.
func New(cfg Config) *Channel {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	graphBase := strings.TrimRight(cfg.GraphBase, "/")
	if graphBase == "" {
		graphBase = DefaultGraphBase
	}
	return &Channel{
		addr:        cfg.Addr,
		path:        path,
		verifyToken: cfg.VerifyToken,
		appSecret:   cfg.AppSecret,
		accessToken: cfg.AccessToken,
		phoneID:     cfg.PhoneNumberID,
		allow:       cfg.Allowlist,
		bus:         cfg.Bus,
		handler:     cfg.Handler,
		graphBase:   graphBase,
		client:      client,
		dedup:       newDedup(dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "whatsapp" }

// Handler exposes the inbound HTTP handler so the daemon (or a test) can mount it.
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

// Start implements channel.Channel: serve the inbound endpoint on cfg.Addr until
// ctx is cancelled. Empty Addr → outbound-only (blocks until ctx is done).
func (c *Channel) Start(ctx context.Context) error {
	if c.addr == "" {
		<-ctx.Done()
		return nil
	}
	srv := &http.Server{
		Addr:              c.addr,
		Handler:           c.Handler(),
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

// --- inbound --------------------------------------------------------------

func (c *Channel) handleInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		c.handleVerify(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !c.verify(r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var wh waWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	// Acknowledge promptly (Meta retries on non-2xx / slow), then process each
	// text message. The dedup guard makes a retried delivery a no-op.
	for _, msg := range wh.textMessages() {
		c.dispatch(r.Context(), msg)
	}
	w.WriteHeader(http.StatusOK)
}

// handleVerify answers Meta's GET subscription handshake: echo hub.challenge when
// hub.verify_token matches the configured token.
func (c *Channel) handleVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if c.verifyToken != "" && q.Get("hub.mode") == "subscribe" &&
		subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(c.verifyToken)) == 1 {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, q.Get("hub.challenge"))
		return
	}
	http.Error(w, "verification failed", http.StatusForbidden)
}

// dispatch runs one inbound text message through the allowlist + handler and sends
// the reply back via the Graph API.
func (c *Channel) dispatch(ctx context.Context, m inboundMsg) {
	if m.from == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.dedup.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind: "whatsapp",
		ChannelID:   m.from,
		Sender:      m.from,
		Text:        m.text,
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(m.from)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.handler == nil {
		return
	}
	reply, err := c.handler(ctx, msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply == "" {
		return
	}
	if err := c.send(ctx, channel.Outbound{ChannelID: m.from, Text: reply, Priority: channel.PriorityNotify}, corr); err != nil && c.bus != nil {
		_, _ = c.bus.Publish(event.Spec{
			Subject: "channel.error.whatsapp", Kind: event.KindChannelOutbound, Actor: "channel-whatsapp",
			CorrelationID: corr, Payload: map[string]any{"error": err.Error(), "channel_id": m.from},
		})
	}
}

// verify checks X-Hub-Signature-256: sha256=<hex HMAC-SHA256(appSecret, body)>,
// constant-time. An empty app secret fails closed.
func (c *Channel) verify(sig string, body []byte) bool {
	if c.appSecret == "" || sig == "" {
		return false
	}
	got := strings.TrimPrefix(sig, "sha256=")
	want := sign(c.appSecret, body)
	return hmac.Equal([]byte(got), []byte(want))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: send a WhatsApp text to out.ChannelID via the
// Graph API, splitting long text. Errors when credentials are missing.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	corr := "chan-" + ulid.New()
	return c.send(ctx, out, corr)
}

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if c.accessToken == "" || c.phoneID == "" {
		return fmt.Errorf("whatsapp: outbound not configured (set AccessToken + PhoneNumberID)")
	}
	endpoint := c.graphBase + "/" + c.phoneID + "/messages"
	for _, chunk := range channel.SplitText(out.Text, waMaxChars) {
		payload, err := json.Marshal(map[string]any{
			"messaging_product": "whatsapp",
			"to":                out.ChannelID,
			"type":              "text",
			"text":              map[string]any{"body": chunk},
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("whatsapp: Graph API returned status %d", resp.StatusCode)
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// --- wire shapes ----------------------------------------------------------

type waWebhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type inboundMsg struct{ from, id, text string }

// textMessages flattens the nested webhook envelope to the text messages it
// carries (non-text message types are ignored — text-only scope).
func (w waWebhook) textMessages() []inboundMsg {
	var out []inboundMsg
	for _, e := range w.Entry {
		for _, ch := range e.Changes {
			for _, m := range ch.Value.Messages {
				if m.Type != "text" {
					continue
				}
				out = append(out, inboundMsg{from: m.From, id: m.ID, text: m.Text.Body})
			}
		}
	}
	return out
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.whatsapp",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-whatsapp",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": msg.ChannelKind,
			"channel_id":   msg.ChannelID,
			"sender":       msg.Sender,
			"text":         msg.Text,
			"allowed":      allowed,
		},
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.whatsapp",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-whatsapp",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// sign returns the hex HMAC-SHA256 of body under secret (Meta's
// X-Hub-Signature-256 scheme).
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// dedup is a small bounded set of recently-seen message ids (Meta retries
// deliveries). Two generations so eviction never forgets every id at once;
// memory bounded at 2×cap. Mirrors the webhook/sms channels.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	prev map[string]struct{}
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

func (d *dedup) seenBefore(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	if _, ok := d.prev[key]; ok {
		return true
	}
	if len(d.seen) >= d.cap {
		d.prev = d.seen
		d.seen = make(map[string]struct{}, d.cap)
	}
	d.seen[key] = struct{}{}
	return false
}
