// SPDX-License-Identifier: MIT

// Package slack is an in-process duplex Channel (SPEC-04 §1) over the Slack
// platform, using net/http + crypto/hmac only — no external dependency. Unlike
// Telegram (which long-polls), Slack pushes events, so the channel SERVES an
// Events API endpoint (POST /slack/events) for inbound and POSTs chat.postMessage
// for outbound. Inbound is verified with Slack's HMAC-SHA256 request signature
// (signing secret) and a timestamp freshness window (replay protection); a fast
// 200 ACK is returned and the agent runs asynchronously, posting its reply when
// done (the standard Slack pattern — Slack retries if not ACKed within 3s).
//
// Security (SPEC-04 §1.7): inbound is an injection surface. The signature gates
// authenticity (only Slack, with the shared secret, can deliver events); an
// Allowlist of channel ids gates who may drive the agent; bot/self messages are
// ignored so the agent never loops on its own replies. Inbound text is data, and
// the agent's tool calls still pass through Edict.
package slack

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
	"strconv"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// DefaultBaseURL is the Slack Web API root (chat.postMessage etc.).
const DefaultBaseURL = "https://slack.com/api"

// EventsPath is the route the channel serves for inbound Events API callbacks.
const EventsPath = "/slack/events"

// maxBody bounds an inbound request body (Slack events are small).
const maxBody = 1 << 20

// signatureWindow is how far an inbound request timestamp may be from now before
// it's rejected as a replay (Slack's documented window is 5 minutes).
const signatureWindow = 5 * time.Minute

// Config constructs a Channel.
type Config struct {
	Token         string // bot token (xoxb-…) for chat.postMessage
	SigningSecret string // Slack app signing secret for inbound verification
	Addr          string // local addr to serve EventsPath (fronted by a tunnel/proxy)
	BaseURL       string // default DefaultBaseURL; override for tests
	HTTPClient    *http.Client
	Allowlist     channel.Allowlist
	Bus           *bus.Bus
	Handler       channel.InboundHandler
}

// Channel is the Slack channel.
type Channel struct {
	token   string
	secret  string
	addr    string
	base    string
	client  *http.Client
	allow   channel.Allowlist
	bus     *bus.Bus
	handler channel.InboundHandler
	now     func() time.Time // injectable clock for signature freshness (tests)
}

// New builds a Channel from cfg.
func New(cfg Config) *Channel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		token:   cfg.Token,
		secret:  cfg.SigningSecret,
		addr:    cfg.Addr,
		base:    base,
		client:  client,
		allow:   cfg.Allowlist,
		bus:     cfg.Bus,
		handler: cfg.Handler,
		now:     time.Now,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "slack" }

// Handler exposes the Events API HTTP handler so the daemon (or a test) can mount
// it on its own mux. Start serves it standalone on cfg.Addr.
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(EventsPath, c.handleEvents)
	return mux
}

// Start implements channel.Channel: serve the Events API endpoint on cfg.Addr
// until ctx is cancelled. Returns nil on a clean shutdown. When Addr is empty the
// channel is outbound-only (Send / Pulse briefs still work); Start blocks until
// ctx is done so the daemon's lifecycle is uniform.
func (c *Channel) Start(ctx context.Context) error {
	if c.addr == "" {
		<-ctx.Done()
		return nil
	}
	srv := &http.Server{Addr: c.addr, Handler: c.Handler()}
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

// --- inbound (Events API) -------------------------------------------------

type slackEnvelope struct {
	Type      string      `json:"type"`      // url_verification | event_callback
	Challenge string      `json:"challenge"` // url_verification handshake
	Event     *slackEvent `json:"event"`
}

type slackEvent struct {
	Type    string `json:"type"`    // "message"
	Channel string `json:"channel"` // C…
	User    string `json:"user"`    // U…
	Text    string `json:"text"`
	TS      string `json:"ts"`
	BotID   string `json:"bot_id"`  // set when the message is from a bot
	Subtype string `json:"subtype"` // set for edits/joins/bot_message/etc.
}

func (c *Channel) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !c.verify(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var env slackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// URL-verification handshake: echo the challenge so Slack accepts the
	// endpoint. (Sent once when the operator configures the Events URL.)
	if env.Type == "url_verification" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": env.Challenge})
		return
	}

	// Everything else: ACK immediately (Slack needs 200 within 3s and retries
	// otherwise) and process asynchronously. A retry delivery (X-Slack-Retry-Num)
	// is ACKed but not reprocessed, so a slow agent run can't be double-handled.
	w.WriteHeader(http.StatusOK)
	if r.Header.Get("X-Slack-Retry-Num") != "" {
		return
	}
	if env.Type != "event_callback" || env.Event == nil {
		return
	}
	ev := *env.Event
	// Only real user messages drive the agent. Ignore bot/self messages and
	// message subtypes (edits, joins, channel_topic, bot_message) — replying to
	// our own posts would loop.
	if ev.Type != "message" || ev.BotID != "" || ev.Subtype != "" || ev.User == "" || ev.Text == "" {
		return
	}
	// Detach from the request context (which ends when we return the ACK); the
	// async run uses a background context so it survives the HTTP response.
	go c.process(context.Background(), ev)
}

// process normalizes one message, enforces the allowlist, runs the handler, and
// posts the reply. Journaled so `agt why`/`agt inbox` can reconstruct it.
func (c *Channel) process(ctx context.Context, ev slackEvent) {
	msg := channel.UnifiedMessage{
		ChannelKind:  "slack",
		ChannelID:    ev.Channel,
		Sender:       ev.User,
		Text:         ev.Text,
		PlatformTSMS: slackTSMillis(ev.TS),
		PlatformMeta: map[string]string{"ts": ev.TS},
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(ev.Channel)
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, Text: "not authorized"}, "")
		return
	}
	if c.handler == nil {
		return
	}
	reply, err := c.handler(ctx, msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply == "" {
		return
	}
	_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, Text: reply, Priority: channel.PriorityNotify}, corr)
}

// verify checks Slack's request signature: v0=HMAC-SHA256(secret, "v0:ts:body"),
// with a timestamp freshness window for replay protection. An empty secret fails
// closed (no inbound without a configured signing secret).
func (c *Channel) verify(ts, sig string, body []byte) bool {
	if c.secret == "" || ts == "" || sig == "" {
		return false
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	delta := c.now().Unix() - n
	if delta < 0 {
		delta = -delta
	}
	if time.Duration(delta)*time.Second > signatureWindow {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

// --- outbound (chat.postMessage) ------------------------------------------

// Send implements channel.Channel (Pulse→Slack sink and out-of-band senders).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

type postMessageResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	body, _ := json.Marshal(map[string]any{"channel": out.ChannelID, "text": out.Text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack chat.postMessage: status %d", resp.StatusCode)
	}
	var pm postMessageResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&pm); err == nil && !pm.OK && pm.Error != "" {
		return fmt.Errorf("slack chat.postMessage: %s", pm.Error)
	}
	c.emitOutbound(out, corr)
	return nil
}

// --- journaling ------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.slack",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-slack",
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
		Subject:       "channel.outbound.slack",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-slack",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "slack",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}

// slackTSMillis converts a Slack ts ("1700000000.000100") to unix millis; 0 on
// parse failure.
func slackTSMillis(ts string) int64 {
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1000)
}
