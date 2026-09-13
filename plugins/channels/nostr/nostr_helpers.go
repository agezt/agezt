// SPDX-License-Identifier: MIT

package nostr

// Nostr helpers + emit: truncate + parseXOnly + broadcast +
// register + unregister + emitInbound + emitOutbound. Carved out
// of nostr.go during the Day 195 god-file split so the main file
// can stay focused on types + lifecycle + inbound handling and
// the publish file can stay focused on outbound Send.
// Public API unchanged.

import (
	"encoding/hex"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)


func truncate(s string) string {
	return strutil.Ellipsis(s, maxChars, "")
}

// parseXOnly turns a 32-byte hex x-only pubkey into a btcec public key.
func parseXOnly(hexKey string) (*btcec.PublicKey, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	return schnorr.ParsePubKey(b)
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


