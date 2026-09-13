// SPDX-License-Identifier: MIT

package feishu

// Feishu helpers: emitInbound + seenBefore. Carved out of
// feishu.go during the Day 193 god-file split so the main file
// can stay focused on types + lifecycle + inbound handling and
// the send file can stay focused on outbound Send + media
// fetch + tenant token.
// Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.feishu",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-feishu",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "feishu", "channel_id": msg.ChannelID,
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

// urlVerification detects the one-time challenge POST and returns (challenge,
// token, true).

