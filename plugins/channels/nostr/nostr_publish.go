// SPDX-License-Identifier: MIT

package nostr

// Nostr outbound Send + publish helpers: Send + publishKind1 +
// publishKind4 + signAndBroadcast. Carved out of nostr.go during
// the Day 195 god-file split so the main file can stay focused on
// types + lifecycle + inbound handling (relayLoop/serveRelay/
// handleFrame/dispatch) and the helpers file can stay focused on
// truncate + parseXOnly + broadcast + register + unregister +
// emitInbound + emitOutbound.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
	btcec "github.com/btcsuite/btcd/btcec/v2"
)

// Send implements channel.Channel: publish out.Text as a standalone note.
func (c *Channel) Send(_ context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	return c.publishKind1(text, [][]string{}, "chan-"+ulid.New())
}

// publishKind1 builds, signs and broadcasts a kind-1 (public note) event.
func (c *Channel) publishKind1(text string, tags [][]string, corr string) error {
	if tags == nil {
		tags = [][]string{}
	}
	ev := nostrEvent{Pubkey: c.pubHex, CreatedAt: time.Now().Unix(), Kind: 1, Tags: tags, Content: truncate(text)}
	return c.signAndBroadcast(ev, text, corr)
}

// publishKind4 sends an encrypted DM (NIP-04) to recipient, journaling the
// plaintext so the operator can read what was sent.
func (c *Channel) publishKind4(recip *btcec.PublicKey, recipHex, text, corr string) error {
	enc, err := nip04Encrypt(c.priv, recip, truncate(text))
	if err != nil {
		return err
	}
	ev := nostrEvent{Pubkey: c.pubHex, CreatedAt: time.Now().Unix(), Kind: 4, Tags: [][]string{{"p", recipHex}}, Content: enc}
	return c.signAndBroadcast(ev, text, corr)
}

// signAndBroadcast signs ev and queues it to every connected relay. journalText
// is the human-readable text recorded in the outbound event (plaintext for DMs).
func (c *Channel) signAndBroadcast(ev nostrEvent, journalText, corr string) error {
	if err := ev.sign(c.priv); err != nil {
		return err
	}
	frame, err := json.Marshal([]any{"EVENT", ev})
	if err != nil {
		return err
	}
	if c.broadcast(frame) == 0 {
		return fmt.Errorf("nostr: no connected relay to publish to")
	}
	c.emitOutbound(channel.Outbound{ChannelID: c.pubHex, Text: journalText, Priority: channel.PriorityNotify}, corr)
	return nil
}
