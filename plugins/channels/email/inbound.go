// SPDX-License-Identifier: MIT

// Email channel: inbound lifecycle (startInbound + prime + poll + dispatch + emit + seenBefore) + mail parsing (parseMail + extractText + decodeBody).
// Code extracted from inbound.go during the Day-125 god-file split.
// Public API unchanged.
package email


import (
	"context"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)


const (
	inboxPollDefault = 60 * time.Second
	maxInboxBytes    = 1 << 20 // bound a fetched message
	dedupCap         = 2048
)

// inboundMail is one fetched message, normalized.
type inboundMail struct {
	from      string // bare sender address (allowlist + reply target)
	subject   string
	body      string
	messageID string
}

// startInbound polls the configured mailbox until ctx is cancelled. Returns
// immediately (caller falls back to blocking) when no inbox is configured.
func (c *Channel) startInbound(ctx context.Context) bool {
	if c.inboxAddr == "" || c.handler == nil {
		return false
	}
	c.prime(ctx) // skip backlog: IMAP relies on \Seen; POP3 records current UIDLs
	t := time.NewTicker(c.pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-t.C:
			mails, err := c.poll(ctx)
			if err != nil {
				continue // transient; retry next tick
			}
			for _, m := range mails {
				c.dispatchInbound(ctx, m)
			}
		}
	}
}

func (c *Channel) prime(ctx context.Context) {
	if c.inboxProto == "pop3" {
		// Record the UIDLs already present so only mail arriving from now on is read.
		if uidls, err := c.popUIDLs(ctx); err == nil {
			c.smu.Lock()
			for _, u := range uidls {
				c.seen[u] = struct{}{}
			}
			c.smu.Unlock()
		}
	}
	// IMAP: nothing to prime — SEARCH UNSEEN + \Seen marking is the cursor.
}

func (c *Channel) poll(ctx context.Context) ([]inboundMail, error) {
	if c.inboxProto == "pop3" {
		return c.pollPOP3(ctx)
	}
	return c.pollIMAP(ctx)
}

func (c *Channel) dispatchInbound(ctx context.Context, m inboundMail) {
	if m.from == "" || strings.TrimSpace(m.body) == "" && strings.TrimSpace(m.subject) == "" {
		return
	}
	if m.messageID != "" && c.seenBefore("mid:"+m.messageID) {
		return
	}
	text := m.subject
	if strings.TrimSpace(m.body) != "" {
		if text != "" {
			text += "\n\n"
		}
		text += m.body
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "email",
		ChannelID:    m.from,
		Sender:       m.from,
		Text:         text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(m.from)
	c.emitInbound(msg, corr, allowed)
	if !allowed {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	if strings.TrimSpace(rep.Text) == "" {
		return
	}
	// Reply over SMTP to the sender (who is allowlisted). Subject threads as Re:.
	_ = c.Send(ctx, channel.Outbound{
		ChannelID: m.from,
		Text:      rep.Text,
		Priority:  channel.PriorityNotify,
	})
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.email",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-email",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "email", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

func (c *Channel) seenBefore(id string) bool {
	c.smu.Lock()
	defer c.smu.Unlock()
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

// --- IMAP -----------------------------------------------------------------

