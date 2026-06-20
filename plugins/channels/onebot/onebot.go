// SPDX-License-Identifier: MIT

// Package onebot is a two-way channel for any OneBot v11-compatible gateway —
// the de-facto standard HTTP protocol behind QQ bots (go-cqhttp, NapCat,
// Lagrange) and unofficial WeChat bridges (wcf / wechatbot). QQ's personal
// accounts and WeChat have no first-party bot API, so a self-hosted OneBot
// gateway is the realistic path (the same "bring a local gateway" model as the
// WhatsApp WAHA channel). One engine backs both: Config.Kind ("qq" / "wechat")
// only sets the channel name (cf. the IRC channel backing Twitch).
//
// Inbound: the gateway POSTs message events to this channel's Addr+Path
// (optionally HMAC-SHA1 signed via X-Signature). Outbound + replies call the
// gateway's HTTP API (/send_msg). An empty allowlist is fail-closed; without an
// Addr the channel is send-only.
package onebot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	maxBody       = 1 << 20
	obMaxChars    = 4000
	dedupCapacity = 2048
)

// Config configures a OneBot channel.
type Config struct {
	Kind        string // "qq" or "wechat" (channel name only)
	APIBase     string // gateway HTTP API base, e.g. http://localhost:5700
	AccessToken string // optional bearer token for the gateway API + inbound
	Secret      string // optional HMAC-SHA1 secret verifying inbound X-Signature
	Allowlist   channel.Allowlist
	Bus         *bus.Bus
	Handler     channel.InboundHandler
	Addr        string // optional host:port to serve the inbound webhook; blank = outbound-only
	Path        string // inbound route (default /<kind>)
	HTTPClient  *http.Client
}

// Channel is the OneBot surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	client  *http.Client
	// mediaClient fetches attacker-supplied CQ media URLs. It is SSRF-guarded
	// (blocks loopback/private/link-local/metadata, re-validates every redirect
	// hop) because, unlike apiBase, the URL comes straight from an inbound message.
	mediaClient *http.Client

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a OneBot channel, applying defaults.
func New(cfg Config) *Channel {
	cfg.Kind = strings.TrimSpace(strings.ToLower(cfg.Kind))
	if cfg.Kind == "" {
		cfg.Kind = "qq"
	}
	if cfg.Path == "" {
		cfg.Path = "/" + cfg.Kind
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{
		cfg:         cfg,
		path:        cfg.Path,
		apiBase:     strings.TrimRight(cfg.APIBase, "/"),
		client:      client,
		mediaClient: netguard.New().HTTPClient(30 * time.Second),
		seen:        make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return c.cfg.Kind }

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
	sender string // user_id, the allowlist key
	target string // reply target: "private:<id>" or "group:<id>"
	text   string
	id     string // message_id for dedup
	media  []obMedia
}

// obMedia is one inbound media segment extracted from CQ codes.
type obMedia struct {
	kind string // "image" | "audio"
	url  string
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
	if c.cfg.Secret != "" && !validSignature(c.cfg.Secret, body, r.Header.Get("X-Signature")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
	if m, ok := parseEvent(body); ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || (strings.TrimSpace(m.text) == "" && len(m.media) == 0) {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  c.cfg.Kind,
		ChannelID:    m.target,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.sender)
	// Inbound media: fetch each CQ image/record URL (allowlisted senders only) so
	// images reach a vision model and voice clips are transcribed.
	if allowed {
		for _, md := range m.media {
			if du := c.fetchMedia(ctx, md.url); du != "" {
				if md.kind == "audio" {
					msg.Audio = append(msg.Audio, du)
				} else {
					msg.Images = append(msg.Images, du)
				}
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
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.target, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to a OneBot target via /send_msg. out.ChannelID is
// "private:<id>" or "group:<id>"; a bare id is treated as a private chat.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("%s: send requires a target (private:<id> or group:<id>)", c.cfg.Kind)
	}
	if text == "" {
		return nil
	}
	if c.apiBase == "" {
		return fmt.Errorf("%s: gateway API base not configured", c.cfg.Kind)
	}
	mtype, id := splitTarget(target)
	for _, chunk := range channel.SplitText(text, obMaxChars) {
		if err := c.sendOne(ctx, mtype, id, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound." + c.cfg.Kind, Kind: event.KindChannelOutbound, Actor: "channel-" + c.cfg.Kind,
			Payload: map[string]any{"channel_kind": c.cfg.Kind, "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, mtype, id, text string) error {
	payload := map[string]any{"message_type": mtype, "message": text}
	// Numeric ids (QQ) go as int64; non-numeric ids (some WeChat gateways) as string.
	var idVal any = id
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		idVal = n
	}
	if mtype == "group" {
		payload["group_id"] = idVal
	} else {
		payload["user_id"] = idVal
	}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/send_msg"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: gateway returned status %d", c.cfg.Kind, resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound." + c.cfg.Kind,
		Kind:          event.KindChannelInbound,
		Actor:         "channel-" + c.cfg.Kind,
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": c.cfg.Kind, "channel_id": msg.ChannelID,
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

func splitTarget(target string) (mtype, id string) {
	if i := strings.IndexByte(target, ':'); i >= 0 {
		t := target[:i]
		if t == "group" || t == "private" {
			return t, target[i+1:]
		}
	}
	return "private", target
}

// validSignature checks X-Signature = "sha1=" + hex(HMAC-SHA1(secret, body)).
func validSignature(secret string, body []byte, header string) bool {
	header = strings.TrimSpace(strings.TrimPrefix(header, "sha1="))
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(header)) == 1
}

// parseEvent reads a OneBot v11 message event: {post_type:"message",
// message_type, message_id, user_id, group_id, raw_message/message}.
func parseEvent(body []byte) (inbound, bool) {
	var e struct {
		PostType    string      `json:"post_type"`
		MessageType string      `json:"message_type"`
		MessageID   json.Number `json:"message_id"`
		UserID      json.Number `json:"user_id"`
		GroupID     json.Number `json:"group_id"`
		RawMessage  string      `json:"raw_message"`
		Message     string      `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return inbound{}, false
	}
	if e.PostType != "" && e.PostType != "message" {
		return inbound{}, false
	}
	text := e.RawMessage
	if text == "" {
		text = e.Message
	}
	user := e.UserID.String()
	target := "private:" + user
	if e.MessageType == "group" && e.GroupID.String() != "" && e.GroupID.String() != "0" {
		target = "group:" + e.GroupID.String()
	}
	clean, media := extractCQMedia(text)
	return inbound{
		sender: user,
		target: target,
		text:   strings.TrimSpace(clean),
		id:     e.MessageID.String(),
		media:  media,
	}, true
}

var cqRe = regexp.MustCompile(`\[CQ:(image|record),([^\]]*)\]`)
var cqURLRe = regexp.MustCompile(`url=([^,\]]+)`)

// fetchMedia downloads a media URL referenced by a CQ code and returns it as an
// inline data: URL. Best-effort: returns "" on any failure.
func (c *Channel) fetchMedia(ctx context.Context, mediaURL string) string {
	if mediaURL == "" {
		return ""
	}
	// The URL is attacker-controlled (it arrived in an inbound CQ code). Require
	// http(s) and fetch only through the SSRF-guarded client, which rejects
	// loopback/private/link-local/metadata targets on the initial dial and on
	// every redirect hop.
	u, err := url.Parse(mediaURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return ""
	}
	resp, err := c.mediaClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return ""
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// extractCQMedia pulls image/record media URLs out of a OneBot raw_message's CQ
// codes and returns the text with those codes removed. CQ entity escapes are
// decoded for the URL.
func extractCQMedia(raw string) (string, []obMedia) {
	var media []obMedia
	for _, m := range cqRe.FindAllStringSubmatch(raw, -1) {
		kind := "image"
		if m[1] == "record" {
			kind = "audio"
		}
		if u := cqURLRe.FindStringSubmatch(m[2]); u != nil {
			media = append(media, obMedia{kind: kind, url: cqUnescape(u[1])})
		}
	}
	return cqRe.ReplaceAllString(raw, ""), media
}

func cqUnescape(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&#91;", "[", "&#93;", "]", "&#44;", ",")
	return r.Replace(s)
}
