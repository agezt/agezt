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
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
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
	if !c.verify(r.Header.Get("X-Webhook-Secret")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
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

// verify constant-time compares the X-Webhook-Secret header against the
// configured shared secret. An empty configured secret fails closed (no
// unauthenticated inbound) — the factory gates the listener on the ADDR, not on
// the secret, so this is the layer that has to hold the invariant.
func (c *Channel) verify(got string) bool {
	if c.cfg.Secret == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(c.cfg.Secret)) == 1
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
