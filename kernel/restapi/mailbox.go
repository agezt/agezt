// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"strconv"

	"github.com/agezt/agezt/kernel/contract/boardapi"
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
func (s *Server) SetMailbox(st boardapi.Store, notify func(m boardapi.Message, corr string)) {
	s.board = st
	s.boardNotify = notify
}

// mailbox returns the wired store or writes a 503 and reports false.
func (s *Server) mailbox(w http.ResponseWriter) (boardapi.Store, bool) {
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

// --- POST + GET /api/v1/mailbox/messages ---

// mailboxAckRequest is the body of an ack: whose inbox to clear.
type mailboxAckRequest struct {
	By string `json:"by"`
}
