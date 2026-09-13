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
	"context"
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
	mediaType  string // "image" | "audio" for media messages (id is the content id)
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
	if !validSignature(c.cfg.Secret, body, r.Header.Get("X-Line-Signature")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	for _, m := range parseWebhook(body) {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.userID == "" || (strings.TrimSpace(m.text) == "" && m.mediaType == "") {
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
	// Inbound media: fetch the message content (allowlisted senders only) so a
	// voice clip is transcribed and an image reaches a vision model.
	if allowed && m.mediaType != "" {
		if du := c.fetchContent(ctx, m.id, m.mediaType); du != "" {
			if m.mediaType == "audio" {
				msg.Audio = []string{du}
			} else {
				msg.Images = []string{du}
			}
		}
	}
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
