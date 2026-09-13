// SPDX-License-Identifier: MIT

package restapi

// Mailbox message handlers: handleMailboxMessages +
// handleMailboxInbox + handleMailboxMessageSub. Carved out of
// mailbox.go during the Day 190 god-file split so the main file
// can stay focused on lifecycle setters + auth + view helpers and
// the watch file can stay focused on the SSE watch stream.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
)

func (s *Server) handleMailboxMessages(w http.ResponseWriter, r *http.Request) {
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		msgs := st.Read(r.URL.Query().Get("topic"), mailboxLimit(r))
		views := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			views = append(views, mailMsgView(m))
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": views, "count": len(views)})

	case http.MethodPost:
		var req mailboxSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				writeErr(w, http.StatusRequestEntityTooLarge, "request_too_large",
					"request body exceeds the size limit")
				return
			}
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Text) == "" {
			writeErr(w, http.StatusBadRequest, "invalid_request", "text is required")
			return
		}
		now := time.Now().UnixMilli()
		var m board.Message
		var err error
		switch {
		case strings.TrimSpace(req.ReplyTo) != "":
			// A reply goes back to the asker on the original topic (the board
			// tool's op=reply semantics), so the asker's replies view finds it.
			orig, found := st.Get(strings.TrimSpace(req.ReplyTo))
			if !found {
				writeErr(w, http.StatusNotFound, "not_found", "no message with id "+req.ReplyTo)
				return
			}
			m, err = st.Send(board.Message{
				Topic: orig.Topic, From: req.From, To: orig.From, ReplyTo: orig.ID, Text: req.Text,
			}, now)
		case req.Help:
			m, err = st.HelpRequest(req.From, req.To, req.Text, now)
		case strings.TrimSpace(req.To) == board.Everyone:
			m, err = st.Broadcast(req.From, req.Text, now)
		case strings.TrimSpace(req.To) != "":
			topic := strings.TrimSpace(req.Topic)
			if topic == "" {
				topic = "dm"
			}
			m, err = st.Send(board.Message{Topic: topic, From: req.From, To: req.To, Text: req.Text}, now)
		default:
			if strings.TrimSpace(req.Topic) == "" {
				writeErr(w, http.StatusBadRequest, "invalid_request",
					`a message needs a "topic" (post), a "to" (DM, or "*" to broadcast), a "reply_to", or "help": true`)
				return
			}
			m, err = st.Post(req.Topic, req.From, req.Text, now)
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		corr := strings.TrimSpace(req.CorrelationID)
		if corr == "" {
			corr = strings.TrimSpace(r.Header.Get("X-Agezt-Correlation"))
		}
		if s.boardNotify != nil {
			s.boardNotify(m, corr)
		}
		resp := map[string]any{"message": mailMsgView(m)}
		if corr != "" {
			resp["correlation_id"] = corr
		}
		writeJSON(w, http.StatusCreated, resp)

	default:
		methodNotAllowed(w, "GET, POST")
	}
}

// --- GET /api/v1/mailbox/inbox ---

func (s *Server) handleMailboxInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name is required (whose inbox)")
		return
	}
	all := r.URL.Query().Get("all") == "true"
	msgs := st.Inbox(name, mailboxLimit(r), all)
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, mailMsgView(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "waiting": views, "count": len(views)})
}

// --- GET /api/v1/mailbox/messages/{id}/replies, POST .../{id}/ack ---

func (s *Server) handleMailboxMessageSub(w http.ResponseWriter, r *http.Request) {
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/mailbox/messages/"), "/")
	id, action, found := strings.Cut(rest, "/")
	if !found || id == "" {
		writeErr(w, http.StatusNotFound, "not_found", "use /api/v1/mailbox/messages/{id}/replies or .../{id}/ack")
		return
	}
	switch action {
	case "replies":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		msgs := st.Replies(id, mailboxLimit(r))
		views := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			views = append(views, mailMsgView(m))
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "replies": views, "count": len(views)})

	case "ack":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			By string `json:"by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
			return
		}
		if strings.TrimSpace(req.By) == "" {
			writeErr(w, http.StatusBadRequest, "invalid_request", `"by" is required (whose inbox to clear)`)
			return
		}
		_, foundMsg, err := st.Ack(id, req.By)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		if !foundMsg {
			writeErr(w, http.StatusNotFound, "not_found", "no message with id "+id)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"acked": true, "id": id, "by": strings.TrimSpace(req.By)})

	default:
		writeErr(w, http.StatusNotFound, "not_found", "unknown action "+action+" (replies|ack)")
	}
}

// --- GET /api/v1/mailbox/watch (SSE) ---

// mailboxKeepalive is how often the watch stream emits an SSE comment frame so
// idle connections survive proxies and dead peers are detected. The watch is
// open-ended (unlike a run stream), so it can sit silent for hours otherwise.
const mailboxKeepalive = 25 * time.Second

// boardPostedPayload is the metadata a board.posted event carries (the daemon
// journals no message text — watchers fetch the body from the store by id).
