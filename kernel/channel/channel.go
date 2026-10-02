// SPDX-License-Identifier: MIT

// Package channel is the in-process channel machinery: the inbound Allowlist,
// the panic Guard, the process-wide manifest/liveness registry, conversation
// history and message splitting. The canonical messaging types every channel
// normalizes to (SPEC-04 §1.3) and the Channel interface a duplex messaging
// surface implements are declared in kernel/contract/channelapi and aliased
// here. The point of the normalization is that agents, the
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
	"strings"

	"github.com/agezt/agezt/kernel/contract/channelapi"
)

// Channel contract types live in kernel/contract/channelapi
// (architecture/21 W1.1). These aliases keep every existing channel.X reference
// compiling with identical types; new code should import channelapi directly.
type (
	UnifiedMessage = channelapi.UnifiedMessage
	Priority       = channelapi.Priority
	Attachment     = channelapi.Attachment
	Outbound       = channelapi.Outbound
	Reply          = channelapi.Reply
	InboundHandler = channelapi.InboundHandler
	Channel        = channelapi.Channel
)

const (
	PriorityInfo   = channelapi.PriorityInfo
	PriorityNotify = channelapi.PriorityNotify
	PriorityUrgent = channelapi.PriorityUrgent
)

// Allowlist gates which chat ids may drive the agent. An empty allowlist
// denies everyone (fail-closed) — a channel with no configured recipients is
// outbound-only, which is the safe default for "I added a bot token but
// haven't said who's allowed to command it yet".
type Allowlist struct {
	ids  map[string]struct{}
	fold bool // compare case-insensitively (NewFoldedAllowlist)
}

// NewFoldedAllowlist is NewAllowlist for identifiers that compare
// case-insensitively — email addresses. NewAllowlist stays exact, because chat
// ids on other platforms can be case-significant.
func NewFoldedAllowlist(ids []string) Allowlist {
	folded := make([]string, len(ids))
	for i, id := range ids {
		folded[i] = strings.ToLower(id)
	}
	a := NewAllowlist(folded)
	a.fold = true
	return a
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

// Allows reports whether chatID may drive the agent.
func (a Allowlist) Allows(chatID string) bool {
	id := strings.TrimSpace(chatID)
	if a.fold {
		id = strings.ToLower(id)
	}
	_, ok := a.ids[id]
	return ok
}

// Empty reports whether the allowlist gates everyone out (outbound-only).
func (a Allowlist) Empty() bool { return len(a.ids) == 0 }
