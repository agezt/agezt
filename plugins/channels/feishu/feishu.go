// SPDX-License-Identifier: MIT

// Feishu channel: types + lifecycle + receive + send + emit + helpers.
// Code extracted from feishu.go during the Day-119 god-file split.
// Public API unchanged.
package feishu

import (
	"context"
	"crypto/subtle"
	"encoding/json"
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
	sender    string // open_id, the allowlist key
	chatID    string // reply target
	text      string
	id        string // event/message id for dedup
	messageID string // message_id, for the resources endpoint
	fileKey   string // image_key / file_key of an inbound media message
	mediaType string // "image" | "audio"
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
		if !c.validToken(tok) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		return
	}
	m, tok, ok := parseEvent(body)
	if !c.validToken(tok) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	if ok {
		c.dispatch(r.Context(), m)
	}
}

// validToken constant-time compares the event's "token" against the configured
// verification token. An empty configured token fails closed (no unverified
// inbound) — an operator who set an ADDR but skipped AGEZT_FEISHU_VERIFY_TOKEN
// must not end up accepting anonymous events.
func (c *Channel) validToken(tok string) bool {
	if c.cfg.VerifyToken == "" || tok == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(c.cfg.VerifyToken)) == 1
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || (strings.TrimSpace(m.text) == "" && m.fileKey == "") {
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
	// Inbound media: fetch the message resource (allowlisted senders only) so an
	// image reaches a vision model and a voice clip is transcribed.
	if allowed && m.fileKey != "" {
		if du := c.fetchResource(ctx, m.messageID, m.fileKey, m.mediaType); du != "" {
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
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.chatID, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to a Feishu chat (out.ChannelID, else the default chat).
