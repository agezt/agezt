// SPDX-License-Identifier: MIT

package sdk

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

// Mail is one message on the daemon's shared mailbox (the inter-agent message
// board, M937): agents and SDK apps leave messages for each other by name,
// broadcast announcements, and read what waits for them.
type Mail struct {
	// ID identifies the message; pass it to Reply, Ack, or Replies.
	ID string
	// Topic groups messages ("dm" for direct messages by default).
	Topic string
	// From is the sender's name (an agent slug or an app's chosen name).
	From string
	// To is the recipient's name; "*" means every inbox (a broadcast).
	To string
	// ReplyTo links an answer back to the message it answers.
	ReplyTo string
	// Text is the message body.
	Text string
	// Help marks an assistance request: it stays open until someone answers.
	Help bool
	// At is when the message was sent.
	At time.Time
}

// MailDraft describes a message to send. Text is required. Addressing:
//   - To = an agent/app name → a direct message (Topic defaults to "dm")
//   - To = "*"               → a broadcast to every inbox
//   - To empty, Topic set    → a plain topic post
//   - ReplyTo = a message id → a reply (it goes back to the original sender
//     on its topic; To/Topic are ignored)
//   - Help = true            → an assistance request, broadcast or directed
//
// A directed message journals board.dm.<recipient>, so a standing order can
// wake the addressed agent the moment mail lands.
type MailDraft struct {
	From    string
	To      string
	Topic   string
	ReplyTo string
	Text    string
	Help    bool
}

// SendMail leaves a message on the shared mailbox. See MailDraft for the
// addressing rules.
func (c *Client) SendMail(ctx context.Context, d MailDraft) (Mail, error) {
	res, err := c.cp.Call(ctx, controlplane.CmdBoardSend, map[string]any{
		"from": d.From, "to": d.To, "topic": d.Topic, "reply_to": d.ReplyTo,
		"text": d.Text, "help": d.Help,
	})
	if err != nil {
		return Mail{}, err
	}
	m, _ := res["sent"].(map[string]any)
	return parseMail(m), nil
}

// Broadcast sends an announcement to EVERY inbox except the sender's.
func (c *Client) Broadcast(ctx context.Context, from, text string) (Mail, error) {
	return c.SendMail(ctx, MailDraft{From: from, To: "*", Text: text})
}

// Inbox returns what waits for name, newest first: messages addressed to it
// plus broadcasts it didn't send. Answered and acked messages are dropped
// unless includeRead is true. limit <= 0 uses the daemon default (50).
func (c *Client) Inbox(ctx context.Context, name string, includeRead bool, limit int) ([]Mail, error) {
	args := map[string]any{"to": name, "all": includeRead}
	if limit > 0 {
		args["limit"] = limit
	}
	res, err := c.cp.Call(ctx, controlplane.CmdBoardInbox, args)
	if err != nil {
		return nil, err
	}
	return parseMails(res["waiting"]), nil
}

// AckMail marks a message read for one reader: it leaves that reader's inbox
// without a reply being written. Per-reader (a broadcast acked by one agent
// still waits for the others) and idempotent.
func (c *Client) AckMail(ctx context.Context, id, by string) error {
	_, err := c.cp.Call(ctx, controlplane.CmdBoardAck, map[string]any{"id": id, "by": by})
	return err
}

// MailReplies returns the answers to a sent message, oldest first
// (conversation order). limit <= 0 uses the daemon default (50).
func (c *Client) MailReplies(ctx context.Context, id string, limit int) ([]Mail, error) {
	args := map[string]any{"id": id}
	if limit > 0 {
		args["limit"] = limit
	}
	res, err := c.cp.Call(ctx, controlplane.CmdBoardReplies, args)
	if err != nil {
		return nil, err
	}
	return parseMails(res["replies"]), nil
}

// MailMessages returns recent mailbox messages, newest first, optionally
// filtered to one topic (case-insensitive). limit <= 0 uses the daemon
// default (50).
func (c *Client) MailMessages(ctx context.Context, topic string, limit int) ([]Mail, error) {
	args := map[string]any{}
	if topic != "" {
		args["topic"] = topic
	}
	if limit > 0 {
		args["limit"] = limit
	}
	res, err := c.cp.Call(ctx, controlplane.CmdBoardRead, args)
	if err != nil {
		return nil, err
	}
	return parseMails(res["messages"]), nil
}

// parseMails maps a []any of message views to typed Mail values.
func parseMails(raw any) []Mail {
	items, _ := raw.([]any)
	out := make([]Mail, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, parseMail(m))
	}
	return out
}

// parseMail maps one message view to a Mail.
func parseMail(m map[string]any) Mail {
	var out Mail
	out.ID, _ = m["id"].(string)
	out.Topic, _ = m["topic"].(string)
	out.From, _ = m["from"].(string)
	out.To, _ = m["to"].(string)
	out.ReplyTo, _ = m["reply_to"].(string)
	out.Text, _ = m["text"].(string)
	out.Help, _ = m["help"].(bool)
	if ts := intFromAny(m["ts_unix_ms"]); ts > 0 {
		out.At = time.UnixMilli(ts)
	}
	return out
}
