// SPDX-License-Identifier: MIT

// Package nostr is a two-way Nostr channel. It connects to a set of relays over
// WebSocket, subscribes to kind-1 notes that mention the agent's pubkey, and
// replies with signed, threaded kind-1 events (NIP-01 / NIP-10). Outbound-only
// use (Pulse briefs, `agt send`) publishes a standalone note.
//
// Nostr is the one channel that can't ride AGEZT's stdlib-only convention: it
// needs a WebSocket transport (github.com/coder/websocket) and BIP340 schnorr
// signing over secp256k1 (github.com/btcsuite/btcd/btcec) — both added
// deliberately for this channel.
//
// Security (SPEC-04 §1.7): inbound events are data, never kernel instructions.
// Every inbound event's schnorr signature is verified locally against its id and
// author pubkey BEFORE it is trusted — a malicious relay cannot forge an event
// attributed to an allowlisted author. An Allowlist of author pubkeys gates who
// may drive the agent (empty = fail-closed for driving; still journaled), and
// event ids are de-duplicated across relays. The private key signs outbound
// events and never leaves the process.
package nostr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/coder/websocket"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
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
	hexKey := strings.TrimSpace(cfg.PrivKeyHex)
	if strings.HasPrefix(hexKey, "nsec1") {
		return nil, fmt.Errorf("nostr: provide the secret key as 64-char hex, not nsec (bech32 not supported)")
	}
	raw, err := hex.DecodeString(hexKey)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("nostr: PrivKeyHex must be 64 hex chars (32 bytes)")
	}
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

	// Subscribe to kind-1 notes that p-tag us, from now on.
	sub := "agezt-" + ulid.New()
	filter := map[string]any{
		"kinds": []int{1},
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
	if strings.TrimSpace(ev.Content) == "" {
		return
	}
	if c.seenBefore(ev.ID) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "nostr",
		ChannelID:    ev.Pubkey,
		Sender:       ev.Pubkey,
		Text:         ev.Content,
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
	// Reply threaded to the mention (NIP-10): e-tag the root, p-tag the author.
	tags := [][]string{{"e", ev.ID, "", "reply"}, {"p", ev.Pubkey}}
	_ = c.publish(ctx, rep.Text, tags, corr)
}

// Send implements channel.Channel: publish out.Text as a standalone note.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	return c.publish(ctx, text, [][]string{}, "chan-"+ulid.New())
}

// publish builds, signs and broadcasts a kind-1 event to every connected relay.
func (c *Channel) publish(ctx context.Context, text string, tags [][]string, corr string) error {
	if tags == nil {
		tags = [][]string{}
	}
	body := text
	if len(body) > maxChars {
		body = body[:maxChars]
	}
	ev := nostrEvent{
		Pubkey:    c.pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      1,
		Tags:      tags,
		Content:   body,
	}
	if err := ev.sign(c.priv); err != nil {
		return err
	}
	frame, err := json.Marshal([]any{"EVENT", ev})
	if err != nil {
		return err
	}
	sent := c.broadcast(frame)
	if sent == 0 {
		return fmt.Errorf("nostr: no connected relay to publish to")
	}
	c.emitOutbound(channel.Outbound{ChannelID: c.pubHex, Text: text, Priority: channel.PriorityNotify}, corr)
	return nil
}

// broadcast queues frame to every connected relay; returns how many accepted it.
func (c *Channel) broadcast(frame []byte) int {
	c.mu.Lock()
	conns := append([]*relayConn(nil), c.conns...)
	c.mu.Unlock()
	sent := 0
	for _, rc := range conns {
		select {
		case rc.out <- frame:
			sent++
		default: // relay's write queue is full; skip it rather than block
		}
	}
	return sent
}

func (c *Channel) register(rc *relayConn) {
	c.mu.Lock()
	c.conns = append(c.conns, rc)
	c.mu.Unlock()
}

func (c *Channel) unregister(rc *relayConn) {
	c.mu.Lock()
	for i, x := range c.conns {
		if x == rc {
			c.conns = append(c.conns[:i], c.conns[i+1:]...)
			break
		}
	}
	c.mu.Unlock()
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.nostr",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-nostr",
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
		Subject:       "channel.outbound.nostr",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-nostr",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- event model + crypto -------------------------------------------------

type nostrEvent struct {
	ID        string     `json:"id"`
	Pubkey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// serialize produces the NIP-01 canonical form whose sha256 is the event id:
// [0,pubkey,created_at,kind,tags,content] with no extra whitespace and HTML
// escaping disabled (Nostr uses standard minimal JSON string escapes).
func (e *nostrEvent) serialize() []byte {
	tags := e.Tags
	if tags == nil {
		tags = [][]string{}
	}
	arr := []any{0, e.Pubkey, e.CreatedAt, e.Kind, tags, e.Content}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(arr)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// sign computes the id and schnorr signature, filling ID and Sig.
func (e *nostrEvent) sign(priv *btcec.PrivateKey) error {
	sum := sha256.Sum256(e.serialize())
	sig, err := schnorr.Sign(priv, sum[:])
	if err != nil {
		return err
	}
	e.ID = hex.EncodeToString(sum[:])
	e.Sig = hex.EncodeToString(sig.Serialize())
	return nil
}

// verify recomputes the id, checks it matches e.ID, and verifies the schnorr
// signature against the author pubkey. False on any malformed field.
func (e *nostrEvent) verify() bool {
	sum := sha256.Sum256(e.serialize())
	if hex.EncodeToString(sum[:]) != e.ID {
		return false
	}
	pkBytes, err := hex.DecodeString(e.Pubkey)
	if err != nil || len(pkBytes) != 32 {
		return false
	}
	pub, err := schnorr.ParsePubKey(pkBytes)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(e.Sig)
	if err != nil {
		return false
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return false
	}
	return sig.Verify(sum[:], pub)
}

// seenBefore records an event id and reports whether it was already processed.
func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCap {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}
