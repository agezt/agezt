// SPDX-License-Identifier: MIT

// WeCom channel: types + lifecycle + emit/dedup helpers.
// The meatiest request-handling methods (handleInbound +
// dispatch) live in wecom_message.go. Crypto helpers live in
// wecom_crypto.go; the outbound send machinery lives in
// wecom_outbound.go. Code was originally split from wecom.go
// during the Day-112 god-file split. Public API unchanged.
package wecom

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)


const (
	// DefaultPath is the inbound callback route WeCom should POST to.
	DefaultPath    = "/wecom"
	defaultAPIBase = "https://qyapi.weixin.qq.com"
	maxBody        = 1 << 20
	wecomMaxChars  = 2000
	dedupCapacity  = 2048
)

// Config configures the WeCom channel.
type Config struct {
	CorpID     string // corp id
	CorpSecret string // app secret (fetches access_token)
	AgentID    string // app agent id
	Token      string // callback token (signature)
	AESKey     string // callback EncodingAESKey (43 chars)
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound callback; blank = outbound-only
	Path       string // inbound route (default /wecom)
	APIBase    string // WeCom API base (default https://qyapi.weixin.qq.com); overridable for tests
	HTTPClient *http.Client
}

// Channel is the WeCom surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	aesKey  []byte
	client  *http.Client

	tmu      sync.Mutex
	token    string
	tokenExp time.Time

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a WeCom channel, applying defaults. AESKey decode errors leave
// aesKey nil; the channel then rejects inbound (outbound still works).
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
	var key []byte
	if cfg.AESKey != "" {
		if k, err := base64.StdEncoding.DecodeString(cfg.AESKey + "="); err == nil {
			key = k
		}
	}
	return &Channel{
		cfg:     cfg,
		path:    cfg.Path,
		apiBase: base,
		aesKey:  key,
		client:  client,
		seen:    make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "wecom" }

// Start serves the inbound callback when an Addr is set; otherwise blocks.
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

// Handler exposes the inbound callback handler (for embedding in a shared mux).
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

type inbound struct {
	sender    string // FromUserName, the allowlist key
	text      string
	id        string // MsgId for dedup
	mediaID   string // MediaId of an inbound image/voice message
	mediaType string // "image" | "audio"
}


// Send delivers out.Text to a WeCom user (out.ChannelID) via the app message API.

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.wecom",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-wecom",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "wecom", "channel_id": msg.ChannelID,
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

// ---- WXBizMsgCrypt -------------------------------------------------------

// signature is the WeCom message signature: sha1 of the sorted concatenation of
// token, timestamp, nonce and the encrypted payload.
