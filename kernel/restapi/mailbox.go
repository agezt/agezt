// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"strconv"

	"github.com/agezt/agezt/kernel/board"
)


// mailboxDefaultLimit / mailboxMaxLimit bound the list endpoints (mirrors the
// control plane's board limits).
const (
	mailboxDefaultLimit = 50
	mailboxMaxLimit     = 500
)

// SetMailbox wires the daemon's ONE shared board store and its post notifier.
// The store must be the same instance the `board` tool holds — a second
// instance would clobber its last write (each holds the whole message list in
// memory and saves it whole). notify publishes board.posted (nil-safe); corr is
// optional per request so channel/webhook bridges can keep a wake causally
// attached to the inbound event that posted the mailbox message.
func (s *Server) SetMailbox(st *board.Store, notify func(m board.Message, corr string)) {
	s.board = st
	s.boardNotify = notify
}

// mailbox returns the wired store or writes a 503 and reports false.
func (s *Server) mailbox(w http.ResponseWriter) (*board.Store, bool) {
	if s.board == nil {
		writeErr(w, http.StatusServiceUnavailable, "mailbox_unavailable",
			"the mailbox is not available on this daemon")
		return nil, false
	}
	return s.board, true
}

// mailboxLimit reads ?limit=, clamped to 1..mailboxMaxLimit.
func mailboxLimit(r *http.Request) int {
	limit := mailboxDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > mailboxMaxLimit {
		limit = mailboxMaxLimit
	}
	return limit
}

// mailMsgView renders one message for a REST response.
func mailMsgView(m board.Message) map[string]any {
	v := map[string]any{"topic": m.Topic, "text": m.Text, "ts_unix_ms": m.TSMS}
	if m.ID != "" {
		v["id"] = m.ID
	}
	if m.From != "" {
		v["from"] = m.From
	}
	if m.To != "" {
		v["to"] = m.To
	}
	if m.ReplyTo != "" {
		v["reply_to"] = m.ReplyTo
	}
	if m.Help {
		v["help"] = true
	}
	if len(m.AckedBy) > 0 {
		v["acked_by"] = append([]string(nil), m.AckedBy...)
	}
	return v
}

// --- POST + GET /api/v1/mailbox/messages ---

type mailboxSendRequest struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Topic         string `json:"topic"`
	ReplyTo       string `json:"reply_to"`
	Text          string `json:"text"`
	Help          bool   `json:"help"`
	CorrelationID string `json:"correlation_id"`
}

