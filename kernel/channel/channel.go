// SPDX-License-Identifier: MIT

// Package channel defines the canonical messaging types every channel
// normalizes to (SPEC-04 §1.3) and the Channel interface a duplex messaging
// surface implements. The point of the normalization is that agents, the
// Unified Inbox, and Pulse only ever see a UnifiedMessage — adding a 20th
// channel never ripples into them.
//
// Phase 4 ships one in-process channel (Telegram); the interface is the same
// one an out-of-process polyglot channel plugin will satisfy later
// (SPEC-04 §1.6).
//
// Security (SPEC-04 §1.7): a channel is an injection surface. Inbound text is
// data, never kernel instructions; an Allowlist gates who may drive the agent
// at all, and the agent's tool calls still pass through Edict.
package channel

import (
	"context"
	"strings"
)

// UnifiedMessage is the platform-neutral inbound message (SPEC-04 §1.3,
// mirrors .project/agezt.proto §UnifiedMessage). Native concepts map on:
// conversation/thread → ChannelID; body → Text; anything platform-specific is
// preserved in PlatformMeta (never lost, not required by core).
type UnifiedMessage struct {
	ChannelKind  string            `json:"channel_kind"`             // "telegram", "discord", ...
	ChannelID    string            `json:"channel_id"`               // conversation/chat id
	Sender       string            `json:"sender"`                   // platform user id/handle
	Text         string            `json:"text"`                     // message body
	PlatformTSMS int64             `json:"platform_ts_ms,omitempty"` // platform timestamp
	PlatformMeta map[string]string `json:"platform_meta,omitempty"`  // preserved extras
}

// Priority lets the Briefing composer drive delivery urgency (SPEC-04 §1.5).
type Priority string

const (
	PriorityInfo   Priority = "info"
	PriorityNotify Priority = "notify"
	PriorityUrgent Priority = "urgent"
)

// Outbound is a message the kernel hands a channel to deliver.
type Outbound struct {
	ChannelID string   `json:"channel_id"`
	Text      string   `json:"text"`
	Priority  Priority `json:"priority,omitempty"`
}

// InboundHandler turns an inbound message into a reply. The daemon supplies it
// (wired to the agent loop). corr is the correlation the channel minted for
// this exchange, so the handler can run the agent under it and keep the
// channel.inbound/outbound events linked to the agent's task arc. An empty
// reply means "nothing to send back". A non-nil error is surfaced to the user
// as a short failure notice.
type InboundHandler func(ctx context.Context, msg UnifiedMessage, corr string) (reply string, err error)

// Channel is a duplex messaging surface (SPEC-04 §1.2).
type Channel interface {
	// Name identifies the channel kind ("telegram").
	Name() string
	// Start begins listening; inbound messages flow to the handler. Returns
	// when ctx is cancelled (or on a fatal connect error).
	Start(ctx context.Context) error
	// Send delivers an outbound message.
	Send(ctx context.Context, out Outbound) error
}

// Allowlist gates which chat ids may drive the agent. An empty allowlist
// denies everyone (fail-closed) — a channel with no configured recipients is
// outbound-only, which is the safe default for "I added a bot token but
// haven't said who's allowed to command it yet".
type Allowlist struct {
	ids map[string]struct{}
}

// NewAllowlist builds an Allowlist from a slice of chat ids (whitespace
// trimmed; blanks ignored).
func NewAllowlist(ids []string) Allowlist {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			m[id] = struct{}{}
		}
	}
	return Allowlist{ids: m}
}

// ParseAllowlist parses a comma-separated list of chat ids.
func ParseAllowlist(s string) Allowlist {
	return NewAllowlist(strings.Split(s, ","))
}

// Allows reports whether chatID may drive the agent.
func (a Allowlist) Allows(chatID string) bool {
	_, ok := a.ids[strings.TrimSpace(chatID)]
	return ok
}

// Empty reports whether the allowlist gates everyone out (outbound-only).
func (a Allowlist) Empty() bool { return len(a.ids) == 0 }
