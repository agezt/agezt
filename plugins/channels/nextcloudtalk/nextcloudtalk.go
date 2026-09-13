// SPDX-License-Identifier: MIT

// Package nextcloudtalk is a two-way Nextcloud Talk channel over the Talk Bot
// API. Nextcloud POSTs an Activity-Streams "Create" event to this channel's
// Addr+Path when a message arrives in a conversation the bot is in; the channel
// verifies the HMAC signature, runs the agent, and posts the reply back through
// the bot message API. Outbound-only (Pulse briefs, `agt send`) works whenever a
// ServerURL + Secret are configured, even without an inbound Addr.
//
// Security (SPEC-04 §1.7): inbound is signature-verified — Nextcloud sends
// X-Nextcloud-Talk-Signature = hex(HMAC-SHA256(secret, RANDOM || body)) over the
// X-Nextcloud-Talk-Random header value and the raw body; an empty secret fails
// closed (no unsigned inbound). An Allowlist of conversation tokens gates who may
// drive the agent, and message ids are de-duplicated (replay guard). Critically,
// the reply is ALWAYS sent to the operator-configured ServerURL — never to the
// attacker-influenceable X-Nextcloud-Talk-Backend header — so a forged backend
// can't turn the channel into an SSRF/secret-leak primitive.
package nextcloudtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	// DefaultPath is the inbound route the channel serves.
	DefaultPath = "/nextcloudtalk"
	// maxBody bounds an inbound request body.
	maxBody = 1 << 20
	// maxChars chunks long replies (Talk caps a chat message at 32000 chars).
	maxChars = 32000
	// dedupCapacity bounds the replay-guard ring of recently-seen message ids.
	dedupCapacity = 4096
)

// Config configures a Nextcloud Talk channel.
type Config struct {
	// ServerURL is the Nextcloud base URL, e.g. https://cloud.example.com. All
	// outbound replies go here (never the inbound Backend header). Required to send.
	ServerURL string
	// Secret is the shared bot secret used for HMAC signing/verification. Empty
	// disables inbound (fail closed).
	Secret string
	// Addr is the local address to serve the inbound webhook (empty → outbound-only).
	Addr string
	// Path is the inbound route; empty → DefaultPath.
	Path string
	// Allowlist gates which conversation tokens may drive the agent.
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	HTTPClient *http.Client
}

// Channel is the Nextcloud Talk surface.
type Channel struct {
	server  string
	secret  string
	addr    string
	path    string
	allow   channel.Allowlist
	bus     *bus.Bus
	handler channel.InboundHandler
	client  *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a Nextcloud Talk channel, applying defaults.
func New(cfg Config) *Channel {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		server:  strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/"),
		secret:  cfg.Secret,
		addr:    cfg.Addr,
		path:    path,
		allow:   cfg.Allowlist,
		bus:     cfg.Bus,
		handler: cfg.Handler,
		client:  client,
		seen:    make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "nextcloudtalk" }

// Handler exposes the inbound HTTP handler for embedding in a shared mux.
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

// Start serves the inbound webhook on cfg.Addr until ctx is cancelled. An empty
// Addr makes the channel outbound-only (blocks until ctx is done).
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	random := r.Header.Get("X-Nextcloud-Talk-Random")
	if !c.verify(random, r.Header.Get("X-Nextcloud-Talk-Signature"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)

	m, ok := parseActivity(body)
	if !ok {
		return
	}
	channel.Guard(c.bus, "nextcloudtalk", func() { c.dispatch(r.Context(), m) })
}

// inbound is one normalized inbound message.
type inbound struct {
	token  string // conversation token (reply target + allowlist key)
	sender string // actor id, e.g. "users/alice"
	text   string
	id     string // message id, for dedup
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.token == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.token+":"+m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "nextcloudtalk",
		ChannelID:    m.token,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(m.token)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.handler == nil {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" {
		return
	}
	_ = c.send(ctx, m.token, reply, corr)
}

// verify checks hex(HMAC-SHA256(secret, random || body)) == signature, constant
// time. An empty secret or random fails closed.
func (c *Channel) verify(random, sig string, body []byte) bool {
	if c.secret == "" || random == "" || sig == "" {
		return false
	}
	want := c.sign(random, body)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(sig))), []byte(want)) == 1
}

// sign returns hex(HMAC-SHA256(secret, random || data)).
func (c *Channel) sign(random string, data []byte) string {
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(random))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: post out.Text to the conversation token in
// out.ChannelID via the Talk bot message API.
