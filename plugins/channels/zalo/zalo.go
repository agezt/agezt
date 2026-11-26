// SPDX-License-Identifier: MIT

// Package zalo is a two-way Zalo channel over the Official Account (OA) API.
// Zalo POSTs user events to this channel's Addr+Path (optionally verified with
// the OA secret key: sha256(appId + body + timestamp + secret) == X-ZEvent-
// Signature). Replies + proactive briefs use the OA message API with a
// configured access token. An empty allowlist is fail-closed; without an Addr
// the channel is send-only.
package zalo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/contract/channelhookapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/platform/netout"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// DefaultPath is the inbound webhook route Zalo should POST to.
	DefaultPath    = "/zalo"
	defaultAPIBase = "https://openapi.zalo.me"
	maxBody        = 1 << 20
	zaloMaxChars   = 2000
	dedupCapacity  = 2048
)

// Config configures the Zalo channel.
type Config struct {
	AppID       string // OA app id (part of the signature)
	AccessToken string // OA access token (sends)
	Secret      string // OA secret key (verifies inbound signature)
	Allowlist   channel.Allowlist
	Bus         *bus.Bus
	Handler     channel.InboundHandler
	Addr        string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path        string // inbound route (default /zalo)
	APIBase     string // Zalo API base (default https://openapi.zalo.me); overridable for tests
	HTTPClient  *http.Client
}

// Channel is the Zalo surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	client  *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a Zalo channel, applying defaults.
func New(cfg Config) *Channel {
	// The contract and this member must agree on the body's shape: a contract
	// that mistyped a field stops the daemon HERE, instead of the door
	// measuring arriving bodies against a shape the member cannot hold.
	if err := channelhookapi.CheckShape(channelhookapi.MemberZalo, reflect.TypeOf(zaloWebhook{})); err != nil {
		panic("channels/zalo: the contract and the plugin disagree: " + err.Error())
	}
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = netout.OperatorClient(30 * time.Second)
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
func (c *Channel) Name() string { return "zalo" }

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
	sender string // sender.id (user_id), the allowlist key + reply target
	text   string
	id     string // message msg_id for dedup
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
	// Zalo's signature covers the timestamp INSIDE the body, so the timestamp
	// is read from the raw bytes before the body is decoded.
	ts := zaloTimestamp(body)
	if !c.validSignature(body, ts, r.Header.Get("X-ZEvent-Signature")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// The door: the body's shape is named in the contract
	// (kernel/contract/channelhookapi) and measured HERE — after the signature
	// is verified, and before the plugin reads the body and before any answer
	// is written. A mistyped field is refused with the operation registry's
	// words; the member keeps its own voice for a body that is not JSON at
	// all: "bad json", a NEW voice for this member — such a body used to be
	// answered 200 and swallowed whenever it carried a signature.
	shape, _ := channelhookapi.Body(channelhookapi.MemberZalo)
	var wh zaloWebhook
	if err := httpserver.CheckJSONBody(body, shape, channelhookapi.MemberZalo, &wh); err != nil {
		httpserver.RefuseJSONBody(w, err, "bad json")
		return
	}
	// Reject stale timestamps (±5 min) so a captured signed event can't be
	// replayed indefinitely. (msg_id replay is also caught by the dedup ring
	// in dispatch.)
	if !freshTimestamp(ts) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	m, _, ok := parseEvent(wh)
	w.WriteHeader(http.StatusOK)
	if ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || strings.TrimSpace(m.text) == "" {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "zalo",
		ChannelID:    m.sender,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.sender)
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
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.sender, Text: reply, Priority: channel.PriorityNotify})
}

// Send delivers out.Text to a Zalo user (out.ChannelID) via the OA message API.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("zalo: send requires a user_id")
	}
	if text == "" {
		return nil
	}
	for _, chunk := range channel.SplitText(text, zaloMaxChars) {
		if err := c.sendOne(ctx, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.zalo", Kind: event.KindChannelOutbound, Actor: "channel-zalo",
			Payload: map[string]any{"channel_kind": "zalo", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, userID, text string) error {
	payload := map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/v3.0/oa/message"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", c.cfg.AccessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("zalo: send returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.zalo",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-zalo",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "zalo", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCapacity {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}

// ---- wire shapes ---------------------------------------------------------

// zaloWebhook is a Zalo OA event: the event's name, who sent it, the message
// and the timestamp the signature covers.
//
// The shape is NAMED, not anonymous: the contract
// (kernel/contract/channelhookapi) holds the same shape as a JSON Schema, and
// the plugin compares the two when it is built — so a contract that mistyped a
// field stops the daemon instead of quietly disagreeing with the member that
// reads it.
type zaloWebhook struct {
	EventName string `json:"event_name"`
	Sender    struct {
		ID string `json:"id"`
	} `json:"sender"`
	Message struct {
		MsgID string `json:"msg_id"`
		Text  string `json:"text"`
	} `json:"message"`
	Timestamp string `json:"timestamp"`
}

// zaloTimestamp reads the timestamp out of a raw body, for the signature.
//
// Zalo's signature covers the timestamp INSIDE the body, so the timestamp has
// to be read before the body is decoded — and it is read as a RAW value and
// unquoted, so the signature covers the very bytes the vendor signed.
//
// A body that is not JSON at all has no timestamp: the answer is "", and the
// signature then fails, which is what a body that is not JSON deserves.
func zaloTimestamp(body []byte) string {
	var doc struct {
		Timestamp json.RawMessage `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(doc.Timestamp))
	if raw == "" || raw == "null" {
		return ""
	}
	if strings.HasPrefix(raw, `"`) {
		var s string
		if err := json.Unmarshal(doc.Timestamp, &s); err != nil {
			return ""
		}
		return s
	}
	return raw
}

// freshTimestamp reports whether a Zalo epoch-ms timestamp is within ±5 minutes
// of now — the replay window for signed events.
func freshTimestamp(ts string) bool {
	ms, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	if err != nil {
		return false
	}
	d := time.Now().UnixMilli() - ms
	return d <= 5*60*1000 && d >= -5*60*1000
}

// validSignature checks Zalo's signature: sha256(appId + body + timestamp +
// secret), hex, optionally prefixed "mac=". An empty secret fails closed (no
// unsigned inbound) — the factory gates the listener on the ADDR, not on the
// secret, and an empty secret would otherwise make the digest attacker-computable.
func (c *Channel) validSignature(body []byte, timestamp, header string) bool {
	if c.cfg.Secret == "" || strings.TrimSpace(header) == "" {
		return false
	}
	header = strings.TrimSpace(strings.TrimPrefix(header, "mac="))
	h := sha256.New()
	h.Write([]byte(c.cfg.AppID))
	h.Write(body)
	h.Write([]byte(timestamp))
	h.Write([]byte(c.cfg.Secret))
	want := hex.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(header)) == 1
}

// parseEvent reads a Zalo OA event: {event_name, sender:{id}, message:{msg_id,
// text}, timestamp}. Only user text messages are kept. Returns (msg, timestamp, ok).
//
// The event arrives already DECODED and already MEASURED — the door
// (handleInbound) holds it to the contract's schema before it gets here — so
// this function only reads the fields it needs.
func parseEvent(e zaloWebhook) (inbound, string, bool) {
	if e.EventName != "" && !strings.HasPrefix(e.EventName, "user_send_text") {
		return inbound{}, e.Timestamp, false
	}
	return inbound{
		sender: e.Sender.ID,
		text:   strings.TrimSpace(e.Message.Text),
		id:     e.Message.MsgID,
	}, e.Timestamp, true
}
