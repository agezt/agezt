// SPDX-License-Identifier: MIT

// Nostr channel: types + lifecycle + receive + send + emit + helpers.
// Code extracted from nostr.go during the Day-107 god-file split.
// Public API unchanged.
package nostr

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/coder/websocket"
)



const (
	maxChars     = 8000    // generous per-note cap
	readLimit    = 1 << 20 // bound a relay frame (events can be large)
	dialTimeout  = 15 * time.Second
	reconnectGap = 5 * time.Second
	dedupCap     = 4096
)

// Config configures a Nostr channel.
type Config struct {
	// PrivKeyHex is the agent's secret key as 64-char hex (not nsec). Required.
	PrivKeyHex string
	// Relays is the list of wss:// (or ws://) relay URLs to connect to.
	Relays []string
	// Allowlist gates which author pubkeys (hex) may drive the agent.
	Allowlist channel.Allowlist
	Bus       *bus.Bus
	Handler   channel.InboundHandler
}

// Channel is the Nostr surface.
type Channel struct {
	priv    *btcec.PrivateKey
	pubHex  string // x-only pubkey hex (the agent's npub, in hex form)
	relays  []string
	allow   channel.Allowlist
	bus     *bus.Bus
	handler channel.InboundHandler

	mu    sync.Mutex
	conns []*relayConn

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

type relayConn struct {
	url string
	out chan []byte // serialized client→relay frames (single writer drains it)
}

// New constructs a Nostr channel. Returns an error if the private key is missing
// or malformed.
func New(cfg Config) (*Channel, error) {
	hexKey, err := decodeNostrKey(cfg.PrivKeyHex, "nsec")
	if err != nil {
		return nil, err
	}
	raw, _ := hex.DecodeString(hexKey)
	priv, _ := btcec.PrivKeyFromBytes(raw)
	pubHex := hex.EncodeToString(schnorr.SerializePubKey(priv.PubKey())) // 32-byte x-only
	var relays []string
	for _, r := range cfg.Relays {
		if r = strings.TrimSpace(r); r != "" {
			relays = append(relays, r)
		}
	}
	return &Channel{
		priv:    priv,
		pubHex:  pubHex,
		relays:  relays,
		allow:   cfg.Allowlist,
		bus:     cfg.Bus,
		handler: cfg.Handler,
		seen:    make(map[string]struct{}, dedupCap),
	}, nil
}

// PubHex returns the agent's x-only public key in hex (for operator display).
func (c *Channel) PubHex() string { return c.pubHex }

// Name implements channel.Channel.
func (c *Channel) Name() string { return "nostr" }

// Start connects to each relay (one goroutine per relay, reconnecting with a gap)
// and blocks until ctx is cancelled. With no relays it just blocks.
func (c *Channel) Start(ctx context.Context) error {
	if len(c.relays) == 0 {
		<-ctx.Done()
		return nil
	}
	var wg sync.WaitGroup
	for _, url := range c.relays {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			c.relayLoop(ctx, u)
		}(url)
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}

// relayLoop maintains one relay connection: dial → subscribe → pump reads and
// queued writes → reconnect after a gap on failure, until ctx is cancelled.
func (c *Channel) relayLoop(ctx context.Context, url string) {
	for {
		if ctx.Err() != nil {
			return
		}
		c.serveRelay(ctx, url)
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectGap):
		}
	}
}

func (c *Channel) serveRelay(ctx context.Context, url string) {
	dctx, dcancel := context.WithTimeout(ctx, dialTimeout)
	conn, _, err := websocket.Dial(dctx, url, nil)
	dcancel()
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(readLimit)

	rc := &relayConn{url: url, out: make(chan []byte, 16)}
	c.register(rc)
	defer c.unregister(rc)

	// Subscribe to kind-1 mentions and kind-4 encrypted DMs that p-tag us, from now on.
	sub := "agezt-" + ulid.New()
	filter := map[string]any{
		"kinds": []int{1, 4},
		"#p":    []string{c.pubHex},
		"since": time.Now().Unix(),
	}
	reqFrame, _ := json.Marshal([]any{"REQ", sub, filter})
	if err := conn.Write(ctx, websocket.MessageText, reqFrame); err != nil {
		return
	}

	// Reader goroutine: relay→client frames onto msgs (one reader per conn).
	msgs := make(chan []byte, 32)
	go func() {
		defer close(msgs)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			select {
			case msgs <- data:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-msgs:
			if !ok {
				return
			}
			c.handleFrame(ctx, data)
		case frame := <-rc.out:
			if err := conn.Write(ctx, websocket.MessageText, frame); err != nil {
				return
			}
		}
	}
}

// handleFrame parses a relay message; only verified EVENT frames are dispatched.
func (c *Channel) handleFrame(ctx context.Context, data []byte) {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil || len(arr) < 1 {
		return
	}
	var typ string
	if json.Unmarshal(arr[0], &typ) != nil || typ != "EVENT" || len(arr) < 3 {
		return
	}
	var ev nostrEvent
	if json.Unmarshal(arr[2], &ev) != nil {
		return
	}
	if !ev.verify() {
		return // forged or corrupt — never trust the relay's word for it
	}
	channel.Guard(c.bus, "nostr", func() { c.dispatch(ctx, ev) })
}

func (c *Channel) dispatch(ctx context.Context, ev nostrEvent) {
	if ev.Pubkey == c.pubHex {
		return // never react to our own notes (loop guard)
	}
	if c.seenBefore(ev.ID) {
		return
	}
	// Resolve the author pubkey (for ECDH on DMs and for threading).
	their, perr := parseXOnly(ev.Pubkey)
	isDM := ev.Kind == 4
	text := ev.Content
	if isDM {
		if perr != nil {
			return
		}
		dec, derr := nip04Decrypt(c.priv, their, ev.Content)
		if derr != nil {
			return // not a DM we can read (wrong recipient / malformed)
		}
		text = dec
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "nostr",
		ChannelID:    ev.Pubkey,
		Sender:       ev.Pubkey,
		Text:         text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(ev.Pubkey)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.handler == nil {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	if strings.TrimSpace(rep.Text) == "" {
		return
	}
	if isDM {
		// Reply privately, encrypted to the sender (NIP-04).
		_ = c.publishKind4(their, ev.Pubkey, rep.Text, corr)
		return
	}
	// Public reply threaded to the mention (NIP-10): e-tag the root, p-tag the author.
	tags := [][]string{{"e", ev.ID, "", "reply"}, {"p", ev.Pubkey}}
	_ = c.publishKind1(rep.Text, tags, corr)
}

