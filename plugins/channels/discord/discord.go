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
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
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
		token:   cfg.Token,
		pubKey:  pub,
		appID:   cfg.ApplicationID,
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
	if c.addr == "" {
		<-ctx.Done()
		return nil
	}
	srv := &http.Server{Addr: c.addr, Handler: c.Handler(), ReadHeaderTimeout: 10 * time.Second}
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

// --- inbound (Interactions) -----------------------------------------------

type discordInteraction struct {
	ID        string         `json:"id"`
	Type      int            `json:"type"`
	Token     string         `json:"token"` // follow-up webhook token
	ChannelID string         `json:"channel_id"`
	Data      *discordData   `json:"data"`
	Member    *discordMember `json:"member"` // present in a guild
	User      *discordUser   `json:"user"`   // present in a DM
}

type discordData struct {
	Name     string           `json:"name"`
	Options  []discordOption  `json:"options"`
	Resolved *discordResolved `json:"resolved"`
}

type discordOption struct {
	Name  string          `json:"name"`
	Type  int             `json:"type"`
	Value json.RawMessage `json:"value"`
}

// discordResolved holds the full objects referenced by option values. An
// ATTACHMENT option's value is an attachment id that indexes Attachments.
type discordResolved struct {
	Attachments map[string]discordAttachment `json:"attachments"`
}

type discordAttachment struct {
	URL         string `json:"url"`          // public CDN url, no auth needed
	ContentType string `json:"content_type"` // e.g. "image/png"
	Filename    string `json:"filename"`
}

type discordMember struct {
	User *discordUser `json:"user"`
}

type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (in discordInteraction) senderID() string {
	if in.Member != nil && in.Member.User != nil {
		return in.Member.User.ID
	}
	if in.User != nil {
		return in.User.ID
	}
	return ""
}

// optionTypeString is Discord's APPLICATION_COMMAND option type for a STRING
// (https://discord.com/developers/docs/interactions/...): only these carry a
// free-text value. Sub-command / group options (types 1, 2) nest their own
// options and must not be mistaken for the prompt.
const optionTypeString = 3

// optionTypeAttachment is Discord's APPLICATION_COMMAND option type for an
// ATTACHMENT: the option value is an attachment id resolved via
// data.resolved.attachments (M249).
const optionTypeAttachment = 11

// imageAttachments returns the CDN urls + content types of attachment options
// that resolve to an image. Only options the registered command declared as
// ATTACHMENT are considered; a non-image attachment is ignored.
func (in discordInteraction) imageAttachments() []discordAttachment {
	if in.Data == nil || in.Data.Resolved == nil {
		return nil
	}
	var out []discordAttachment
	for _, o := range in.Data.Options {
		if o.Type != optionTypeAttachment {
			continue
		}
		var id string
		if err := json.Unmarshal(o.Value, &id); err != nil || id == "" {
			continue
		}
		att, ok := in.Data.Resolved.Attachments[id]
		if !ok || att.URL == "" || !strings.HasPrefix(att.ContentType, "image/") {
			continue
		}
		out = append(out, att)
	}
	return out
}

// text returns the slash command's prompt. It prefers the option explicitly
// named "prompt" (the registered command's text field) and only considers STRING
// options, so a reordered or additional option can't silently feed the agent the
// wrong field. Falls back to the first STRING option when none is named "prompt".
func (in discordInteraction) text() string {
	if in.Data == nil {
		return ""
	}
	var fallback string
	for _, o := range in.Data.Options {
		if o.Type != optionTypeString {
			continue
		}
		var s string
		if err := json.Unmarshal(o.Value, &s); err != nil || s == "" {
			continue
		}
		if o.Name == "prompt" {
			return s
		}
		if fallback == "" {
			fallback = s
		}
	}
	return fallback
}

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
	if c.handler == nil || (msg.Text == "" && len(in.imageAttachments()) == 0) {
		writeJSON(w, ephemeral("nothing to do"))
		return
	}
	// Defer: Discord shows "Agezt is thinking…"; we follow up when the agent is
	// done. Detach from the request context (it ends with this ACK).
	writeJSON(w, map[string]any{"type": responseDeferred})
	go c.runAndFollowUp(context.Background(), in, msg, corr)
}

// discordAttachMaxRaw bounds a downloaded attachment so the data: URL stays
// within the control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
const discordAttachMaxRaw = 12 << 20

// fetchAttachmentDataURL downloads a Discord attachment (a public CDN url, no
// auth) and returns it as an inline data: URL using the attachment's reported
// content type (M249).
func (c *Channel) fetchAttachmentDataURL(ctx context.Context, att discordAttachment) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, att.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("discord attachment download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, discordAttachMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("discord attachment download: empty")
	}
	if len(data) > discordAttachMaxRaw {
		return "", fmt.Errorf("discord attachment exceeds %d bytes", discordAttachMaxRaw)
	}
	return "data:" + att.ContentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (c *Channel) runAndFollowUp(ctx context.Context, in discordInteraction, msg channel.UnifiedMessage, corr string) {
	// Inbound image attachments (M249): fetch them now (after the fast ACK, so
	// the download never risks the 3s interaction deadline) and attach as data:
	// URLs so a vision model can see them. The allowlist was already enforced
	// before this runs.
	for _, att := range in.imageAttachments() {
		if du, err := c.fetchAttachmentDataURL(ctx, att); err == nil && du != "" {
			msg.Images = append(msg.Images, du)
		}
	}
	reply, err := c.handler(ctx, msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply == "" {
		reply = "(no output)"
	}
	_ = c.followUp(ctx, in.Token, in.ChannelID, reply, corr)
}

// verify checks Discord's Ed25519 request signature over (timestamp || body),
// with a timestamp freshness window for replay protection. A missing/invalid
// public key fails closed.
func (c *Channel) verify(ts, sigHex string, body []byte) bool {
	if len(c.pubKey) != ed25519.PublicKeySize || ts == "" || sigHex == "" {
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
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	msg := make([]byte, 0, len(ts)+len(body))
	msg = append(msg, ts...)
	msg = append(msg, body...)
	return ed25519.Verify(c.pubKey, msg, sig)
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: post a message to a channel via the bot token
// (Pulse→Discord sink and out-of-band senders). Distinct from the interaction
// follow-up path, which authenticates with the interaction token, not the bot.
// discordMaxChars is Discord's per-message content limit (2000 characters). A
// longer message is rejected, so a long answer is split into sequential
// messages rather than lost (M234).
const discordMaxChars = 2000

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	// Discord rejects an empty message; no-op rather than fail (M236).
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	for _, chunk := range channel.SplitText(out.Text, discordMaxChars) {
		body, _ := json.Marshal(map[string]any{"content": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/channels/"+out.ChannelID+"/messages", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bot "+c.token)
		if err := c.do(req); err != nil {
			return err
		}
	}
	c.emitOutbound(out, "")
	return nil
}

// followUp delivers an interaction's answer via the follow-up webhook
// (webhooks/{app}/{token}); the token in the URL authenticates, so no bot header.
// Like Send, it chunks past Discord's 2000-char limit (M234) — each POST to the
// follow-up webhook creates a new message — so a long slash-command answer is
// delivered in sequence rather than rejected and lost.
func (c *Channel) followUp(ctx context.Context, token, channelID, content, corr string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	for _, chunk := range channel.SplitText(content, discordMaxChars) {
		body, _ := json.Marshal(map[string]any{"content": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/webhooks/"+c.appID+"/"+token, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if err := c.do(req); err != nil {
			return err
		}
	}
	c.emitOutbound(channel.Outbound{ChannelID: channelID, Text: content, Priority: channel.PriorityNotify}, corr)
	return nil
}

func (c *Channel) do(req *http.Request) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("discord: status %d", resp.StatusCode)
	}
	return nil
}

// --- helpers / journaling --------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ephemeral builds an immediate, invoker-only message response.
func ephemeral(text string) map[string]any {
	return map[string]any{
		"type": responseMessage,
		"data": map[string]any{"content": text, "flags": flagEphemeral},
	}
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.discord",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-discord",
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
		Subject:       "channel.outbound.discord",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-discord",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "discord",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}
