// SPDX-License-Identifier: MIT

package restapi

// The mailbox surface (M937): the shared inter-agent message board
// (kernel/board) exposed over REST, so apps written with the SDKs — or plain
// curl — can leave messages for agents (or each other), read an inbox, reply,
// and acknowledge, without being inside a run. Same store the `board` tool
// writes; a send here publishes the same board.posted event, so it wakes
// standing orders exactly like an agent's send.
//
// Routes (token-authed like the rest of /api/v1):
//
//	POST /api/v1/mailbox/messages              — send: topic post, DM (to),
//	                                             broadcast (to "*"), reply
//	                                             (reply_to), or help request
//	GET  /api/v1/mailbox/messages?topic=&limit= — recent messages, newest first
//	GET  /api/v1/mailbox/inbox?name=&all=&limit= — what waits for a named
//	                                             agent/app, newest first
//	GET  /api/v1/mailbox/messages/{id}/replies — answers, oldest first
//	POST /api/v1/mailbox/messages/{id}/ack     — mark read for one reader
//	GET  /api/v1/mailbox/watch?name=&topic=    — SSE: new messages as they land
//	GET  /api/v1/mailbox/topics                — topic → message count
//
// The mailbox is daemon-global (one board per daemon), so the X-Agezt-Tenant
// header does not partition it.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
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
// always empty here (no run owns an external send).
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
	return v
}

// --- POST + GET /api/v1/mailbox/messages ---

type mailboxSendRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Topic   string `json:"topic"`
	ReplyTo string `json:"reply_to"`
	Text    string `json:"text"`
	Help    bool   `json:"help"`
}

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
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
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
		if s.boardNotify != nil {
			s.boardNotify(m, "")
		}
		writeJSON(w, http.StatusCreated, map[string]any{"message": mailMsgView(m)})

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
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
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
type boardPostedPayload struct {
	ID    string `json:"id"`
	Topic string `json:"topic"`
	From  string `json:"from"`
	To    string `json:"to"`
	Help  bool   `json:"help"`
}

// mailWatchMatch reports whether a posted message is for this watcher: with a
// name, messages addressed to it (case-insensitive) plus broadcasts it didn't
// send — the live counterpart of Inbox; with a topic, that topic's posts; with
// neither, everything (a firehose tail).
func mailWatchMatch(name, topic string, p boardPostedPayload) bool {
	if name != "" {
		to := strings.ToLower(strings.TrimSpace(p.To))
		directed := to != "" && to == name
		broadcast := p.To == board.Everyone && strings.ToLower(strings.TrimSpace(p.From)) != name
		if !directed && !broadcast {
			return false
		}
	}
	if topic != "" && !strings.EqualFold(strings.TrimSpace(p.Topic), topic) {
		return false
	}
	return true
}

// handleMailboxWatch streams new mailbox messages as SSE `mail` frames the
// moment they land (M938) — the push counterpart of polling the inbox.
// ?name= watches one agent/app's mail (DMs + broadcasts), ?topic= one topic,
// neither tails everything. The mailbox is daemon-global, so this subscribes
// the primary bus regardless of any tenant header.
func (s *Server) handleMailboxWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeErr(w, http.StatusInternalServerError, "stream_unsupported", "streaming unsupported")
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))

	// Subscribe BEFORE the ready frame so no message can slip between them.
	sub, err := s.bus.Subscribe("board.>", 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe_error", err.Error())
		return
	}
	defer sub.Cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	send := func(eventName string, payload any) {
		data, _ := json.Marshal(payload)
		_, _ = w.Write([]byte("event: " + eventName + "\ndata: " + string(data) + "\n\n"))
		flusher.Flush()
	}
	ready := map[string]any{}
	if name != "" {
		ready["name"] = name
	}
	if topic != "" {
		ready["topic"] = topic
	}
	send("ready", ready)

	keepalive := time.NewTicker(mailboxKeepalive)
	defer keepalive.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		case ev, alive := <-sub.C:
			if !alive {
				return
			}
			if ev == nil || ev.Kind != event.KindBoardPosted || len(ev.Payload) == 0 {
				continue
			}
			var p boardPostedPayload
			if json.Unmarshal(ev.Payload, &p) != nil {
				continue
			}
			if !mailWatchMatch(name, topic, p) {
				continue
			}
			// The event carries metadata only; the body lives in the store. The
			// write committed before the publish, so the lookup is reliable — but
			// fall back to the metadata view if the message was already evicted.
			if m, found := st.Get(p.ID); found {
				send("mail", mailMsgView(m))
				continue
			}
			send("mail", map[string]any{
				"id": p.ID, "topic": p.Topic, "from": p.From, "to": p.To, "help": p.Help,
			})
		}
	}
}

// --- GET /api/v1/mailbox/topics ---

func (s *Server) handleMailboxTopics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": st.Topics()})
}
