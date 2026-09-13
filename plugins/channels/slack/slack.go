// SPDX-License-Identifier: MIT

// Slack channel: types + lifecycle + receive flow + Send dispatcher.
// Code extracted from slack.go during the Day-92 god-file split.
// Public API unchanged.
package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
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
