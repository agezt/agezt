// SPDX-License-Identifier: MIT

// slack_emit.go owns the bus-side event emitters for the
// Slack channel: Channel.emitInbound (channel.inbound.slack)
// and Channel.emitOutbound (channel.outbound.slack). The
// actual send-API calls (postMessage / sendFile / Send /
// fetchFileDataURL / send) live in slack_send.go.
package slack

import (
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

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
