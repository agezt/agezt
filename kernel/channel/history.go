// SPDX-License-Identifier: MIT

package channel

// Conversation context (SPEC-04 §1.4). Each inbound message has historically
// driven a fresh, memory-less agent run — so a chat like "what's the weather?"
// → "and tomorrow?" lost all thread. ConversationHistory folds the journal for
// the recent inbound/outbound of ONE conversation (channel kind + id) and
// renders a compact transcript the daemon prepends as the run intent, giving the
// agent multi-turn context. Read-only over the journal (the same source the
// Unified Inbox uses); no new state.

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/event"
)

// EventRanger is the read side of the journal this package needs: walk every
// event in order. *journal.Journal satisfies it; tests pass a fake.
type EventRanger interface {
	Range(fn func(*event.Event) error) error
}

// maxHistoryCharsPerMsg bounds one message's contribution to the transcript so a
// single huge message can't blow the token budget; longer text is truncated.
const maxHistoryCharsPerMsg = 2000

type histTurn struct {
	role string // "user" | "assistant"
	text string
}

// ConversationHistory returns a transcript of the last `limit` messages of the
// caller's conversation in (kind, channelID), oldest first, for use as the run
// intent. It returns "" when limit <= 0, the id is empty, or there is no PRIOR
// context (≤1 message — i.e. only the just-received message), so the caller falls
// back to the raw message text and behavior is unchanged for the first turn.
//
// Privacy in shared channels: a Slack/Discord channel can carry many users. The
// fold isolates per sender — it includes only THIS sender's inbound messages and
// the agent replies that share one of their run correlations — so one user's
// messages never leak into another user's prompt. (sender == "" disables the
// isolation, folding the whole channel; callers pass a real sender id.)
//
// The current inbound message is already journaled by the time a handler runs,
// so it appears as the final `user:` line — the agent replies to it with the
// preceding turns as context.
func ConversationHistory(r EventRanger, kind, channelID, sender string, limit int) string {
	if r == nil || limit <= 0 || channelID == "" {
		return ""
	}
	var turns []histTurn
	mine := map[string]bool{} // correlation ids of this sender's runs
	_ = r.Range(func(e *event.Event) error {
		var p struct {
			ChannelKind string `json:"channel_kind"`
			ChannelID   string `json:"channel_id"`
			Sender      string `json:"sender"`
			Text        string `json:"text"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		if p.ChannelKind != kind || p.ChannelID != channelID || strings.TrimSpace(p.Text) == "" {
			return nil
		}
		switch e.Kind {
		case event.KindChannelInbound:
			// Only this sender's own inbound (the privacy boundary).
			if sender != "" && p.Sender != sender {
				return nil
			}
			if e.CorrelationID != "" {
				mine[e.CorrelationID] = true
			}
			turns = append(turns, histTurn{role: "user", text: clip(p.Text, maxHistoryCharsPerMsg)})
		case event.KindChannelOutbound:
			// Only replies to this sender's runs (paired by correlation), so a
			// reply meant for another user isn't folded in.
			if sender != "" && !mine[e.CorrelationID] {
				return nil
			}
			turns = append(turns, histTurn{role: "assistant", text: clip(p.Text, maxHistoryCharsPerMsg)})
		}
		return nil
	})

	if len(turns) <= 1 {
		return "" // no prior context; caller uses the raw message
	}
	if len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}

	var b strings.Builder
	b.WriteString("[recent conversation, oldest first — reply to the latest user message]\n")
	for _, t := range turns {
		b.WriteString(t.role)
		b.WriteString(": ")
		b.WriteString(t.text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// clip truncates s to at most n bytes on a rune boundary, appending an ellipsis
// marker when cut. Rune-aware so a multibyte sequence (emoji/CJK, common in
// chat) is never split into invalid UTF-8.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	// Back up to the start of the rune that contains byte n.
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
