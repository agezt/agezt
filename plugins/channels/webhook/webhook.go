// SPDX-License-Identifier: MIT

// Package webhook is a vendor-neutral inbound/outbound HTTP channel (SPEC-04 §1):
// any external system can drive an Agezt agent by POSTing a signed JSON message,
// and receives the agent's reply synchronously in the response (and/or async via
// an outbound callback). It's the generic counterpart to the Slack/Discord
// channels — no platform SDK, just a signed JSON envelope:
//
//	POST {Path}
//	X-Agezt-Signature: sha256=<hex HMAC-SHA256(secret, raw-body)>
//	{"channel_id":"...","sender":"...","text":"...","id":"...","ts_ms":...}
//
//	→ 200 {"reply":"...","correlation_id":"..."}
//
// The signature scheme matches Agezt's OUTBOUND webhook dispatcher
// (kernel/webhook), so the two compose and operators already know the format.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Inbound text is data,
// never kernel instructions. The HMAC signature authenticates the sender (empty
// secret fails closed — no unsigned inbound); a freshness window on `ts_ms` plus
// de-duplication on `id` guard against replay; an Allowlist of channel ids gates
// who may drive the agent at all; and the agent's own tool calls still pass
// through Edict. Bodies are length-bounded so a flood can't exhaust memory.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	// DefaultPath is the route the channel serves for inbound deliveries.
	DefaultPath = "/webhook"
	// maxBody bounds an inbound request body.
	maxBody = 1 << 20
	// signatureWindow is how far an inbound `ts_ms` may be from now before the
	// request is rejected as stale (replay protection). Mirrors Slack/Discord.
	signatureWindow = 5 * time.Minute
	// dedupCapacity bounds the replay-guard set of recently-seen message ids.
	dedupCapacity = 4096
)

// Config configures a webhook Channel.
type Config struct {
	// Addr is the local address to serve Path on (e.g. "127.0.0.1:8790"),
	// typically fronted by a tunnel/reverse proxy. Empty → outbound-only.
	Addr string
	// Path is the inbound route; empty defaults to DefaultPath.
	Path string
	// Secret is the HMAC-SHA256 signing key. Empty disables inbound entirely
	// (fail closed — no unsigned commands).
	Secret string
	// Allowlist gates which channel ids may drive the agent.
	Allowlist channel.Allowlist
	// OutboundURL, when set, is where Send POSTs proactive/async messages
	// (Pulse briefs, `agt send`), signed with Secret. Empty → Send errors.
	OutboundURL string
	// Bus journals channel.inbound/outbound events. May be nil.
	Bus *bus.Bus
	// Handler runs the agent for an inbound message. Required for inbound.
	Handler channel.InboundHandler
	// HTTPClient is used for outbound Send; nil → a 30s-timeout client.
	HTTPClient *http.Client
	// now overrides the clock for freshness tests; nil → time.Now.
	now func() time.Time
}

// Channel is the vendor-neutral webhook messaging surface.
type Channel struct {
	addr        string
	path        string
	secret      string
	allow       channel.Allowlist
	outboundURL string
	bus         *bus.Bus
	handler     channel.InboundHandler
	client      *http.Client
	now         func() time.Time
	dedup       *dedup
}

// New constructs a webhook Channel from cfg.
func New(cfg Config) *Channel {
	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	now := cfg.now
	if now == nil {
		now = time.Now
	}
	return &Channel{
		addr:        cfg.Addr,
		path:        path,
		secret:      cfg.Secret,
		allow:       cfg.Allowlist,
		outboundURL: strings.TrimSpace(cfg.OutboundURL),
		bus:         cfg.Bus,
		handler:     cfg.Handler,
		client:      client,
		now:         now,
		dedup:       newDedup(dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "webhook" }

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
// WriteTimeout is left unset — the reply is written after a (possibly slow) agent run.
func (c *Channel) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              c.addr,
		Handler:           c.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// --- inbound --------------------------------------------------------------

type inboundEnvelope struct {
	ChannelID string   `json:"channel_id"`
	Sender    string   `json:"sender"`
	Text      string   `json:"text"`
	ID        string   `json:"id"`     // optional client message id (dedup key)
	TSMS      int64    `json:"ts_ms"`  // optional client timestamp (freshness)
	Images    []string `json:"images"` // optional data: URL attachments
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
	if !c.verify(r.Header.Get("X-Agezt-Signature"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var env inboundEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(env.ChannelID) == "" || strings.TrimSpace(env.Text) == "" {
		http.Error(w, "channel_id and text are required", http.StatusBadRequest)
		return
	}
	// Replay protection: reject a stale timestamp, and de-dupe a repeated id (a
	// captured signed body can otherwise be re-sent within the freshness window).
	if env.TSMS != 0 {
		delta := c.now().UnixMilli() - env.TSMS
		if delta < 0 {
			delta = -delta
		}
		// Compare in integer milliseconds rather than converting to a Duration:
		// time.Duration(delta)*time.Millisecond overflows int64 nanoseconds for a
		// far-future/past timestamp and could wrap negative (which would pass the
		// `> window` check). The timestamp is signed, so this isn't reachable today,
		// but a freshness backstop shouldn't depend on that.
		if delta > int64(signatureWindow/time.Millisecond) {
			http.Error(w, "stale timestamp", http.StatusUnauthorized)
			return
		}
	}
	if env.ID != "" && c.dedup.seenBefore(env.ChannelID+":"+env.ID) {
		writeJSON(w, http.StatusOK, map[string]any{"reply": "", "duplicate": true})
		return
	}

	msg := channel.UnifiedMessage{
		ChannelKind:  "webhook",
		ChannelID:    env.ChannelID,
		Sender:       env.Sender,
		Text:         env.Text,
		Images:       env.Images,
		PlatformTSMS: env.TSMS,
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(env.ChannelID)
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "not authorized", "correlation_id": corr})
		return
	}
	if c.handler == nil {
		writeJSON(w, http.StatusOK, map[string]any{"reply": "", "correlation_id": corr})
		return
	}
	reply, err := c.handler(r.Context(), msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply != "" {
		c.emitOutbound(channel.Outbound{ChannelID: env.ChannelID, Text: reply, Priority: channel.PriorityNotify}, corr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "correlation_id": corr})
}

// verify checks the X-Agezt-Signature header: sha256=<hex HMAC-SHA256(secret,
// body)>, constant-time. An empty secret fails closed (no unsigned inbound).
func (c *Channel) verify(sig string, body []byte) bool {
	if c.secret == "" || sig == "" {
		return false
	}
	got := strings.TrimPrefix(sig, "sha256=")
	want := sign(c.secret, body)
	return hmac.Equal([]byte(got), []byte(want))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: POST the message to the configured
// OutboundURL, signed with the same scheme inbound expects. Errors when no
// OutboundURL is configured (the channel is inbound/synchronous-reply only).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if c.outboundURL == "" {
		return fmt.Errorf("webhook: no outbound URL configured (set it to send async messages)")
	}
	corr := "chan-" + ulid.New()
	return c.send(ctx, out, corr)
}

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	body, err := json.Marshal(map[string]any{
		"channel_id": out.ChannelID,
		"text":       out.Text,
		"priority":   string(out.Priority),
		"ts_ms":      c.now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.outboundURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Agezt-Signature", "sha256="+sign(c.secret, body))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("webhook: outbound POST returned status %d", resp.StatusCode)
	}
	c.emitOutbound(out, corr)
	return nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.webhook",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-webhook",
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
		Subject:       "channel.outbound.webhook",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-webhook",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// sign returns the hex HMAC-SHA256 of body under secret (same as the outbound
// webhook dispatcher in kernel/webhook).
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// dedup is a small bounded set of recently-seen message ids (replay guard). It
// keeps two generations (live + previous) so eviction never forgets every id in
// one shot: a key is dropped only after it ages out of BOTH, bounding memory at
// 2×cap while roughly doubling the replay window's coverage.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	prev map[string]struct{}
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

// seenBefore records key and reports whether it had been seen already (in either
// generation). When the live set fills it rotates to become the previous
// generation and a fresh live set starts — unlike a wholesale clear, which would
// forget every recently-seen id at once and let a captured signed body replay
// (the freshness window only guards replays when the client sends ts_ms; with no
// timestamp this set is the sole replay guard, so it must not flush so coarsely).
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
