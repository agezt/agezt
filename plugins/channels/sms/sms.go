// SPDX-License-Identifier: MIT

// SMS channel: Config + Channel + New + Name + Handler + Start + handleInbound + verify + signedURL + Send + sendOne + emit + writeTwiML + xmlEscape.
// Code extracted from sms.go during the Day-134 god-file split.
// Public API unchanged.
package sms

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// DefaultPath is the inbound route Twilio POSTs to.
	DefaultPath = "/sms"
	// twilioAPIBase is the Programmable Messaging REST root.
	twilioAPIBase = "https://api.twilio.com"
	// maxBody bounds an inbound request body.
	maxBody = 1 << 20
	// smsMaxChars caps each outbound Twilio request body. Twilio accepts up to
	// 1600 chars and segments transparently; we split a touch under that.
	smsMaxChars = 1500
	// dedupCapacity bounds the replay-guard set of recently-seen MessageSids.
	dedupCapacity = 4096
)

// Config configures a Twilio SMS Channel.
type Config struct {
	// Addr is the local address to serve the inbound route on (e.g.
	// "127.0.0.1:8792"), typically fronted by a tunnel/reverse proxy. Empty →
	// outbound-only.
	Addr string
	// Path is the inbound route; empty defaults to DefaultPath.
	Path string
	// AccountSID + AuthToken are the Twilio credentials. AuthToken signs/validates
	// inbound and authenticates outbound; empty AuthToken disables inbound (fail
	// closed — no unsigned commands).
	AccountSID string
	AuthToken  string
	// From is the Twilio phone number outbound messages are sent from (E.164).
	From string
	// PublicURL is the exact public URL Twilio is configured to POST to, used to
	// recompute the request signature (behind a tunnel the local URL differs from
	// what Twilio signed). Empty → reconstruct from the request Host + path.
	PublicURL string
	// Allowlist gates which sender numbers may drive the agent.
	Allowlist channel.Allowlist
	// Bus journals channel.inbound/outbound events. May be nil.
	Bus *bus.Bus
	// Handler runs the agent for an inbound message. Required for inbound.
	Handler channel.InboundHandler
	// APIBase overrides the Twilio REST root (tests point it at a mock). Empty →
	// twilioAPIBase.
	APIBase string
	// HTTPClient is used for outbound Send; nil → a 30s-timeout client.
	HTTPClient *http.Client
}

// Channel is the Twilio SMS messaging surface.
type Channel struct {
	addr      string
	path      string
	sid       string
	token     string
	from      string
	publicURL string
	allow     channel.Allowlist
	bus       *bus.Bus
	handler   channel.InboundHandler
	apiBase   string
	client    *http.Client
	dedup     *dedup
}

// New constructs an SMS Channel from cfg.
func New(cfg Config) *Channel {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	apiBase := strings.TrimRight(cfg.APIBase, "/")
	if apiBase == "" {
		apiBase = twilioAPIBase
	}
	return &Channel{
		addr:      cfg.Addr,
		path:      path,
		sid:       cfg.AccountSID,
		token:     cfg.AuthToken,
		from:      cfg.From,
		publicURL: strings.TrimSpace(cfg.PublicURL),
		allow:     cfg.Allowlist,
		bus:       cfg.Bus,
		handler:   cfg.Handler,
		apiBase:   apiBase,
		client:    client,
		dedup:     newDedup(dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "sms" }

// Handler exposes the inbound HTTP handler so the daemon (or a test) can mount it
// on its own mux. Start serves it standalone on cfg.Addr.
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

// Start implements channel.Channel: serve the inbound endpoint on cfg.Addr until
// ctx is cancelled. Empty Addr → outbound-only (blocks until ctx is done so the
// daemon's lifecycle is uniform).
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
	// Bound the body, then parse the form from the captured bytes (ParseForm would
	// otherwise consume r.Body unbounded).
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !c.verify(r, form) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	from := strings.TrimSpace(form.Get("From"))
	text := strings.TrimSpace(form.Get("Body"))
	sid := form.Get("MessageSid")
	if from == "" || text == "" {
		writeTwiML(w, "")
		return
	}
	// Twilio retries a webhook on timeout; de-dupe the MessageSid so a retried
	// delivery doesn't drive the agent twice.
	if sid != "" && c.dedup.seenBefore(sid) {
		writeTwiML(w, "")
		return
	}

	msg := channel.UnifiedMessage{
		ChannelKind: "sms",
		ChannelID:   from,
		Sender:      from,
		Text:        text,
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(from)
	c.emitInbound(msg, corr, allowed)
	if !allowed {
		// Fail closed: no agent run, no reply (empty TwiML, 200 so Twilio doesn't
		// retry). The refusal is journaled via the allowed=false inbound event.
		writeTwiML(w, "")
		return
	}
	if c.handler == nil {
		writeTwiML(w, "")
		return
	}
	rep, err := c.handler(r.Context(), msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply != "" {
		c.emitOutbound(channel.Outbound{ChannelID: from, Text: reply, Priority: channel.PriorityNotify}, corr)
	}
	writeTwiML(w, reply)
}

// verify authenticates an inbound request with the X-Twilio-Signature header:
// base64(HMAC-SHA1(authToken, fullURL + concat of sorted form key+value)). An
// empty auth token fails closed.
func (c *Channel) verify(r *http.Request, form url.Values) bool {
	if c.token == "" {
		return false
	}
	got := r.Header.Get("X-Twilio-Signature")
	if got == "" {
		return false
	}
	want := twilioSignature(c.token, c.signedURL(r), form)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// signedURL is the URL Twilio signed: the configured PublicURL when set,
// otherwise reconstructed from the request (best-effort when not behind a
// path-rewriting proxy).
func (c *Channel) signedURL(r *http.Request) string {
	if c.publicURL != "" {
		return c.publicURL
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	} else if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

// twilioSignature computes Twilio's request signature: the URL followed by each
// POST param's key and value, sorted by key, HMAC-SHA1'd under the auth token and
// base64-encoded. (https://www.twilio.com/docs/usage/security#validating-requests)
