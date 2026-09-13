// SPDX-License-Identifier: MIT

// Package discord is an in-process duplex Channel (SPEC-04 §1) over Discord,
// using net/http + crypto/ed25519 only — no external dependency, no Gateway
// WebSocket. Free-form Discord messages require the Gateway (a persistent
// WebSocket), which would pull in a dependency; instead this channel drives the
// agent through Discord's HTTP **Interactions** endpoint (a slash command such
// as `/agezt prompt:<text>`). Discord POSTs the interaction to a URL the channel
// SERVES (POST /discord/interactions); the channel verifies Discord's Ed25519
// request signature, ACKs with a DEFERRED response within 3s ("Agezt is
// thinking…"), runs the agent asynchronously, and delivers the answer with a
// follow-up webhook message. Outbound briefs (Pulse) post via the bot token to
// channels/{id}/messages.
//
// This is the same channel.Channel shape as Telegram (long-poll) and Slack
// (HMAC webhook) — only the transport and signature scheme differ, proving the
// abstraction generalizes: Telegram pulls, Slack/Discord push, Slack signs with
// HMAC-SHA256, Discord signs with Ed25519.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. The Ed25519 signature
// gates authenticity (only Discord, holding the app's private key, can deliver a
// valid interaction); an empty/invalid public key fails closed. An Allowlist of
// channel ids gates who may drive the agent. Inbound text is data, and the
// agent's tool calls still pass through Edict.

package discord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)



// DefaultBaseURL is the Discord HTTP API root (v10).
const DefaultBaseURL = "https://discord.com/api/v10"

// InteractionsPath is the route the channel serves for inbound interactions.
const InteractionsPath = "/discord/interactions"

// maxBody bounds an inbound request body (interactions are small).
const maxBody = 1 << 20

// signatureWindow is how far an inbound request timestamp may be from now before
// it's rejected as a replay. Discord interactions are real-time (a 3s response
// budget), so a 5-minute window is generous and still closes the replay door.
const signatureWindow = 5 * time.Minute

// Discord interaction types (request).
const (
	interactionPing    = 1 // Discord verifying the endpoint
	interactionCommand = 2 // APPLICATION_COMMAND (a slash command)
)

// Discord interaction-response types.
const (
	responsePong     = 1  // reply to a PING
	responseMessage  = 4  // CHANNEL_MESSAGE_WITH_SOURCE (immediate)
	responseDeferred = 5  // DEFERRED_CHANNEL_MESSAGE_WITH_SOURCE ("thinking…")
	flagEphemeral    = 64 // message visible only to the invoking user
)

// Config constructs a Channel.
type Config struct {
	Token         string // bot token, for outbound channels/{id}/messages
	PublicKey     string // app public key (hex) for Ed25519 inbound verification
	ApplicationID string // app id, for follow-up webhooks/{app}/{token}
	Addr          string // local addr to serve InteractionsPath (fronted by a proxy)
	BaseURL       string // default DefaultBaseURL; override for tests
	HTTPClient    *http.Client
	Allowlist     channel.Allowlist
	Bus           *bus.Bus
	Handler       channel.InboundHandler
}

// Channel is the Discord channel.
type Channel struct {
	token   string
	pubKey  ed25519.PublicKey
	appID   string
	addr    string
	base    string
	client  *http.Client
	allow   channel.Allowlist
	bus     *bus.Bus
	handler channel.InboundHandler
	now     func() time.Time // injectable clock for signature freshness (tests)
	// attachURLOK validates an attachment URL before the server-side fetch (H-001).
	// Defaults to validDiscordAttachmentURL (https + Discord-CDN host); injectable
	// so tests can point the download at a local httptest server.
	attachURLOK func(string) error
	// baseCtx is the daemon-lifetime context: async inbound runs detach from the
	// short-lived HTTP request context but stay tied to baseCtx so a clean shutdown
	// cancels them after the drain window instead of leaving them to be killed by
	// process exit. Start sets it; New defaults it to context.Background.
	baseCtx context.Context
}

// New builds a Channel from cfg. An unparseable/short public key leaves
// verification fail-closed (no inbound is ever accepted).
func New(cfg Config) *Channel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var pub ed25519.PublicKey
	if b, err := hex.DecodeString(cfg.PublicKey); err == nil && len(b) == ed25519.PublicKeySize {
		pub = ed25519.PublicKey(b)
	}
	return &Channel{
		token:       cfg.Token,
		pubKey:      pub,
		appID:       cfg.ApplicationID,
		addr:        cfg.Addr,
		base:        base,
		client:      client,
		allow:       cfg.Allowlist,
		bus:         cfg.Bus,
		handler:     cfg.Handler,
		now:         time.Now,
		baseCtx:     context.Background(),
		attachURLOK: validDiscordAttachmentURL,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "discord" }

// Handler exposes the Interactions HTTP handler so the daemon (or a test) can
// mount it on its own mux. Start serves it standalone on cfg.Addr.
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(InteractionsPath, c.handleInteractions)
	return mux
}

// Start implements channel.Channel: serve the Interactions endpoint on cfg.Addr
// until ctx is cancelled. When Addr is empty the channel is outbound-only (Send /
// Pulse briefs still work); Start blocks until ctx is done so the daemon's
// lifecycle is uniform.
func (c *Channel) Start(ctx context.Context) error {
	c.baseCtx = ctx // async inbound runs follow daemon lifetime, not the request
	if c.addr == "" {
		<-ctx.Done()
		return nil
	}
	srv := c.newHTTPServer()
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

// newHTTPServer builds the inbound HTTP server with slow-loris timeouts (M431):
// ReadHeaderTimeout + ReadTimeout bound the header and body read so a client can't
// hold a handler goroutine open by dripping bytes; IdleTimeout caps keep-alive idle.
func (c *Channel) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              c.addr,
		Handler:           c.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// --- inbound (Interactions) -----------------------------------------------

func (c *Channel) handleInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !c.verify(r.Header.Get("X-Signature-Timestamp"), r.Header.Get("X-Signature-Ed25519"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var in discordInteraction
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	switch in.Type {
	case interactionPing:
		// Endpoint-verification handshake: reply PONG so Discord accepts the URL.
		writeJSON(w, map[string]any{"type": responsePong})
	case interactionCommand:
		c.handleCommand(w, in)
	default:
		writeJSON(w, ephemeral("unsupported interaction"))
	}
}

// handleCommand normalizes one slash command, enforces the allowlist, ACKs with
// a deferred response, and runs the agent asynchronously (a follow-up webhook
// delivers the reply). Journaled so `agt why`/`agt inbox` can reconstruct it.
func (c *Channel) handleCommand(w http.ResponseWriter, in discordInteraction) {
	msg := channel.UnifiedMessage{
		ChannelKind:  "discord",
		ChannelID:    in.ChannelID,
		Sender:       in.senderID(),
		Text:         in.text(),
		PlatformMeta: map[string]string{"interaction_id": in.ID},
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(in.ChannelID)
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		writeJSON(w, ephemeral("not authorized"))
		return
	}
	if c.handler == nil || (msg.Text == "" && len(in.imageAttachments()) == 0 && len(in.audioAttachments()) == 0) {
		writeJSON(w, ephemeral("nothing to do"))
		return
	}
	// Defer: Discord shows "Agezt is thinking…"; we follow up when the agent is
	// done. Detach from the request context (it ends with this ACK).
	writeJSON(w, map[string]any{"type": responseDeferred})
	go channel.Guard(c.bus, "discord", func() { c.runAndFollowUp(c.baseCtx, in, msg, corr) })
}

// discordAttachMaxRaw bounds a downloaded attachment so the data: URL stays
// within the control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
