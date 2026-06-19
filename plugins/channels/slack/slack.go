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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

// dedupCapacity bounds the replay-guard set (recent message keys). Slack events
// are low-frequency; a few thousand entries covers the 5-minute window cheaply.
const dedupCapacity = 4096

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
	dedup   *dedup           // replay guard: recently-processed message keys
	// baseCtx is the daemon-lifetime context: async inbound runs detach from the
	// short-lived HTTP request context (the handler returns immediately) but stay
	// tied to baseCtx so a clean shutdown cancels them after the drain window
	// instead of leaving them to be killed by process exit. Start sets it; New
	// defaults it to context.Background so a handler driven directly in tests still
	// works.
	baseCtx context.Context
}

// dedup is a small bounded set of recently-seen message keys. The HMAC signature
// proves authenticity but its freshness window (5 min) still permits replay of a
// captured signed body without the retry header; keying on the immutable message
// ts gives exactly-once processing within that window. Bounded FIFO so it can't
// grow without limit.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	ring []string
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

// seenBefore records key and reports whether it had already been seen.
func (d *dedup) seenBefore(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = struct{}{}
	d.ring = append(d.ring, key)
	if len(d.ring) > d.cap {
		old := d.ring[0]
		d.ring = d.ring[1:]
		delete(d.seen, old)
	}
	return false
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
		dedup:   newDedup(dedupCapacity),
		baseCtx: context.Background(),
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
// WriteTimeout is left unset — a reply is sent after a (possibly slow) agent run.
func (c *Channel) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              c.addr,
		Handler:           c.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// --- inbound (Events API) -------------------------------------------------

type slackEnvelope struct {
	Type      string      `json:"type"`      // url_verification | event_callback
	Challenge string      `json:"challenge"` // url_verification handshake
	Event     *slackEvent `json:"event"`
}

type slackEvent struct {
	Type     string      `json:"type"`    // "message"
	Channel  string      `json:"channel"` // C…
	User     string      `json:"user"`    // U…
	Text     string      `json:"text"`
	TS       string      `json:"ts"`
	ThreadTS string      `json:"thread_ts"` // set when the message lives in a thread (M885)
	BotID    string      `json:"bot_id"`    // set when the message is from a bot
	Subtype  string      `json:"subtype"`   // set for edits/joins/bot_message/etc.
	Files    []slackFile `json:"files"`     // shared-file attachments
}

// slackFile is an inbound file attachment. url_private requires the bot token in
// an Authorization header to download; mimetype tells us whether it's an image.
type slackFile struct {
	URLPrivate string `json:"url_private"`
	Mimetype   string `json:"mimetype"`
	Name       string `json:"name"`
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
	// Replay guard: the signature's freshness window still permits replay of a
	// captured signed body (without the retry header) within 5 minutes. Key on the
	// immutable channel+ts so each message drives at most one run.
	if c.dedup.seenBefore(ev.Channel + ":" + ev.TS) {
		return
	}
	// Detach from the request context (which ends when we return the ACK); the
	// async run uses a background context so it survives the HTTP response.
	go channel.Guard(c.bus, "slack", func() { c.process(c.baseCtx, ev) })
}

// process normalizes one message, enforces the allowlist, runs the handler, and
// posts the reply. Journaled so `agt why`/`agt inbox` can reconstruct it.
func (c *Channel) process(ctx context.Context, ev slackEvent) {
	msg := channel.UnifiedMessage{
		ChannelKind:  "slack",
		ChannelID:    ev.Channel,
		ThreadID:     ev.ThreadTS, // M885: a Slack thread is its own conversation
		Sender:       ev.User,
		Text:         ev.Text,
		PlatformTSMS: slackTSMillis(ev.TS),
		PlatformMeta: map[string]string{"ts": ev.TS},
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(ev.Channel)
	// Inbound image files (M248): fetch each shared image as a data: URL so a
	// vision model can see it. Only for allowlisted senders — never download a
	// file referenced by an unauthorized sender.
	if allowed && len(ev.Files) > 0 {
		for _, f := range ev.Files {
			if !strings.HasPrefix(f.Mimetype, "image/") || f.URLPrivate == "" {
				continue
			}
			if du, err := c.fetchFileDataURL(ctx, f.URLPrivate, f.Mimetype); err == nil && du != "" {
				msg.Images = append(msg.Images, du)
			}
		}
	}
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, Text: "not authorized"}, "")
		return
	}
	if c.handler == nil {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" && len(rep.Attachments) == 0 {
		return
	}
	_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, ThreadID: msg.ThreadID, Text: reply, Attachments: rep.Attachments, Priority: channel.PriorityNotify}, corr)
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
	// Integer-seconds comparison: time.Duration(delta)*time.Second overflows int64
	// nanoseconds for a far-off timestamp and could wrap negative (passing the
	// `> window` check). Signed input makes this unreachable today; a freshness
	// backstop shouldn't rely on that.
	if delta > int64(signatureWindow/time.Second) {
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

// slackMaxChars is Slack's per-message text limit (40000 characters). A longer
// message is rejected, so a long answer is split into sequential messages rather
// than lost (M240) — same treatment as Telegram/Discord (M234/M235).
const slackMaxChars = 40000

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	// Slack rejects an empty message (errors with "no_text"); no-op rather than
	// fail, covering the Send path and whitespace-only answers (M236).
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil
	}
	for _, chunk := range channel.SplitText(out.Text, slackMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		if err := c.postMessage(ctx, out.ChannelID, out.ThreadID, chunk); err != nil {
			return err
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendFile(ctx, out.ChannelID, out.ThreadID, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// sendFile uploads an attachment via Slack's external-upload flow:
// files.getUploadURLExternal → POST bytes to the upload URL →
// files.completeUploadExternal (which shares it into the channel/thread).
func (c *Channel) sendFile(ctx context.Context, channelID, threadTS string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	// 1) get an upload URL + file id.
	form := url.Values{"filename": {fn}, "length": {strconv.Itoa(len(att.Data))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/files.getUploadURLExternal", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	var up struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&up)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if !up.OK || up.UploadURL == "" {
		return fmt.Errorf("slack getUploadURLExternal: %s", up.Error)
	}
	// 2) POST the bytes to the upload URL (multipart field "file").
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fn)
	if err != nil {
		return err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	ureq, err := http.NewRequestWithContext(ctx, http.MethodPost, up.UploadURL, &buf)
	if err != nil {
		return err
	}
	ureq.Header.Set("Content-Type", mw.FormDataContentType())
	uresp, err := c.client.Do(ureq)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(uresp.Body, 8<<10))
	uresp.Body.Close()
	if uresp.StatusCode/100 != 2 {
		return fmt.Errorf("slack upload: status %d", uresp.StatusCode)
	}
	// 3) complete + share into the channel/thread.
	complete := map[string]any{
		"files":      []map[string]string{{"id": up.FileID, "title": fn}},
		"channel_id": channelID,
	}
	if threadTS != "" {
		complete["thread_ts"] = threadTS
	}
	cbody, _ := json.Marshal(complete)
	creq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/files.completeUploadExternal", bytes.NewReader(cbody))
	if err != nil {
		return err
	}
	creq.Header.Set("Content-Type", "application/json; charset=utf-8")
	creq.Header.Set("Authorization", "Bearer "+c.token)
	cresp, err := c.client.Do(creq)
	if err != nil {
		return err
	}
	defer cresp.Body.Close()
	var cr struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(cresp.Body, maxBody)).Decode(&cr); err != nil {
		return err
	}
	if !cr.OK {
		return fmt.Errorf("slack completeUploadExternal: %s", cr.Error)
	}
	return nil
}

// postMessage delivers one chat.postMessage and verifies Slack's app-level ok.
// Slack returns HTTP 200 even on application errors ({"ok":false,"error":…}), so
// the body must be decoded and ok checked — a decode failure or ok=false is a
// FAILED send, not a delivered one.
func (c *Channel) postMessage(ctx context.Context, channelID, threadTS, text string) error {
	fields := map[string]any{"channel": channelID, "text": text}
	if threadTS != "" {
		fields["thread_ts"] = threadTS // M885: reply inside the originating thread
	}
	body, _ := json.Marshal(fields)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&pm); err != nil {
		return fmt.Errorf("slack chat.postMessage: decode response: %w", err)
	}
	if !pm.OK {
		reason := pm.Error
		if reason == "" {
			reason = "ok=false"
		}
		return fmt.Errorf("slack chat.postMessage: %s", reason)
	}
	return nil
}

// --- journaling ------------------------------------------------------------

// slackFileMaxRaw bounds a downloaded file so the data: URL stays within the
// control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
const slackFileMaxRaw = 12 << 20

// fetchFileDataURL downloads a Slack url_private attachment (which requires the
// bot token in an Authorization header) and returns it as an inline data: URL.
// The bytes are read here in the channel, where the token lives, and handed
// onward as a self-describing data: URL the vision providers emit natively
// (M248).
func (c *Channel) fetchFileDataURL(ctx context.Context, urlPrivate, mimetype string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPrivate, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("slack file download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, slackFileMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("slack file download: empty")
	}
	if len(data) > slackFileMaxRaw {
		return "", fmt.Errorf("slack file exceeds %d bytes", slackFileMaxRaw)
	}
	return "data:" + mimetype + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	payload := map[string]any{
		"channel_kind": msg.ChannelKind,
		"channel_id":   msg.ChannelID,
		"sender":       msg.Sender,
		"text":         msg.Text,
		"allowed":      allowed,
	}
	if msg.ThreadID != "" {
		payload["thread_id"] = msg.ThreadID // M885: history folds per thread
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.slack",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-slack",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	payload := map[string]any{
		"channel_kind": "slack",
		"channel_id":   out.ChannelID,
		"text":         out.Text,
		"priority":     string(out.Priority),
	}
	if out.ThreadID != "" {
		payload["thread_id"] = out.ThreadID // M885
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.slack",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-slack",
		CorrelationID: corr,
		Payload:       payload,
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
