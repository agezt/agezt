// SPDX-License-Identifier: MIT

// Package sms is an inbound/outbound SMS channel over Twilio's Programmable
// Messaging API (SPEC-04 §1). An allowlisted phone number can drive an Agezt
// agent by texting the daemon's Twilio number; the agent replies in the same
// thread. Proactive messages (Pulse briefs, `agt send`) go out via Twilio's
// REST API.
//
// Inbound: Twilio POSTs application/x-www-form-urlencoded to the configured
// route (From, Body, MessageSid, …). The handler authenticates the request with
// the X-Twilio-Signature header (base64 HMAC-SHA1 over the request URL + sorted
// POST params, keyed by the account auth token) — empty auth token fails closed,
// so no unsigned inbound. The reply is returned synchronously as TwiML
// (<Response><Message>…</Message></Response>), Twilio's native reply path.
//
// Outbound: POST to /2010-04-01/Accounts/{SID}/Messages.json, form-encoded
// (To/From/Body), HTTP Basic auth (AccountSID:AuthToken). Long replies are split
// with channel.SplitText.
//
// Security (SPEC-04 §1.7): inbound text is data, never kernel instructions; the
// signature authenticates Twilio; an Allowlist of sender numbers gates who may
// drive the agent (fail-closed); a dedup set on MessageSid guards retries; bodies
// are length-bounded. The agent's own tool calls still pass through Edict.
package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
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
	reply, err := c.handler(r.Context(), msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
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
func twilioSignature(token, fullURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		b.WriteString(k)
		// Multi-valued params concatenate their values in order.
		for _, v := range form[k] {
			b.WriteString(v)
		}
	}
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: send an SMS to out.ChannelID (a phone number)
// via the Twilio REST API, splitting long text. Errors when no From number or
// credentials are configured.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	if c.from == "" || c.sid == "" || c.token == "" {
		return fmt.Errorf("sms: outbound not configured (set AccountSID, AuthToken, From)")
	}
	corr := "chan-" + ulid.New()
	for _, chunk := range channel.SplitText(out.Text, smsMaxChars) {
		if err := c.sendOne(ctx, out.ChannelID, chunk); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

func (c *Channel) sendOne(ctx context.Context, to, body string) error {
	endpoint := c.apiBase + "/2010-04-01/Accounts/" + url.PathEscape(c.sid) + "/Messages.json"
	form := url.Values{}
	form.Set("To", to)
	form.Set("From", c.from)
	form.Set("Body", body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.sid, c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("sms: Twilio API returned status %d", resp.StatusCode)
	}
	return nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.sms",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-sms",
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
		Subject:       "channel.outbound.sms",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-sms",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// writeTwiML writes a TwiML response. An empty reply yields a bare <Response/>
// (acknowledge, no message). The reply text is XML-escaped.
func writeTwiML(w http.ResponseWriter, reply string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if reply == "" {
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Response></Response>`)
		return
	}
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Message>`+xmlEscape(reply)+`</Message></Response>`)
}

// xmlEscape escapes the five XML predefined entities so a reply can't break the
// TwiML envelope.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// dedup is a small bounded set of recently-seen MessageSids (replay/retry guard).
// It keeps two generations so eviction never forgets every id at once: a key is
// dropped only after it ages out of both, bounding memory at 2×cap. Mirrors the
// webhook channel's guard.
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
