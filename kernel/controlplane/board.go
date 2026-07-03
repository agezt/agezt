// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
)

// boardReadDefaultLimit / boardReadMaxLimit bound CmdBoardRead.
const (
	boardReadDefaultLimit = 50
	boardReadMaxLimit     = 500
)

// boardReader returns a store for READ handlers: the daemon's shared instance
// when wired (SetBoard), else a fresh read-only Open — writes are atomic, so a
// fresh Open sees the latest committed state.
func (s *Server) boardReader() (*board.Store, error) {
	if s.boardStore != nil {
		return s.boardStore, nil
	}
	return board.Open(filepath.Join(s.baseDir, "board"))
}

// boardWriter returns the store for WRITE handlers: ONLY the shared instance.
// A fresh Open here would race the `board` tool's instance — each holds the
// whole message list in memory and saves it whole, so the last writer would
// silently drop the other's message.
func (s *Server) boardWriter() (*board.Store, bool) {
	return s.boardStore, s.boardStore != nil
}

// boardLimitArg reads the clamped limit argument shared by the board handlers.
func boardLimitArg(args map[string]any) int {
	limit := boardReadDefaultLimit
	if raw, ok := args["limit"]; ok {
		if v, ok := raw.(float64); ok && v > 0 {
			limit = int(v)
		}
	}
	if limit > boardReadMaxLimit {
		limit = boardReadMaxLimit
	}
	return limit
}

// boardMsgView renders one message for a control-plane response.
func boardMsgView(m board.Message) map[string]any {
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

// handleBoardRead serves CmdBoardRead: a read-only view of the shared inter-agent
// message board so the Web UI can show agents talking to each other. The board
// is the `board` tool's store (kernel/board) under <baseDir>/board; we Open it
// fresh per request — writes are atomic, so a fresh Open sees the latest
// committed state without sharing the tool's in-process instance.
func (s *Server) handleBoardRead(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	topic, _ := req.Args["topic"].(string)

	// Cursor pagination (M-pending follow-up): the SPA's Board / Inbox /
	// ChannelSessions views poll this on every render; for a busy board the
	// payload is large enough to make the panel slow. Cursor encodes the
	// (TSMS, ID) of the LAST entry on the previous page; server skips
	// entries strictly newer-or-equal. ID tie-breaks when TSMS collides.
	var cursorTS int64
	var cursorID string
	cursorOK := false
	if raw, ok := req.Args["cursor"].(string); ok && raw != "" {
		tsStr, id, _ := strings.Cut(raw, ":")
		if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
			cursorTS, cursorID, cursorOK = ts, id, true
		}
	}
	limit := boardLimitArg(req.Args)
	// Always ask the store for the full set — Read() truncates internally,
	// which would hide the rows the cursor filter is supposed to skip, and
	// would prevent page 1 from emitting a next_cursor when the total is
	// exactly one full page. The handler applies limit + cursor itself.
	storeLimit := 0

	// Addressed messaging (M788/M791): the views carry id/to/reply_to so the
	// console threads DMs and replies.
	msgs := st.Read(topic, storeLimit)
	// st.Read() sorts by TSMS DESC with stable insertion order — two messages
	// with identical TSMS keep their insertion order, so "newest" within a
	// millisecond is not actually at position 0. Re-sort by (TSMS, ID) DESC
	// here so the pagination cursor's tie-break (ID) is well-defined.
	sort.SliceStable(msgs, func(i, j int) bool {
		if msgs[i].TSMS != msgs[j].TSMS {
			return msgs[i].TSMS > msgs[j].TSMS
		}
		return msgs[i].ID > msgs[j].ID
	})
	total := len(msgs)
	if cursorOK {
		filtered := msgs[:0]
		for _, m := range msgs {
			if m.TSMS > cursorTS {
				continue
			}
			if m.TSMS == cursorTS && m.ID >= cursorID {
				continue
			}
			filtered = append(filtered, m)
		}
		msgs = filtered
	}
	var nextCursor string
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
		last := msgs[limit-1]
		nextCursor = strconv.FormatInt(last.TSMS, 10) + ":" + last.ID
	}
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, boardMsgView(m))
	}

	result := map[string]any{"messages": views, "topics": st.Topics(), "count": len(views), "total": total}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleBoardHelp serves CmdBoardHelp: the still-open (unanswered) help requests
// agents have raised (M849), newest first — the "who needs help" view for the
// Web UI and an overseer agent. Read-only, fresh Open per request like the read.
func (s *Server) handleBoardHelp(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	msgs := st.OpenHelp(boardLimitArg(req.Args))
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, boardMsgView(m))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"open_help": views, "count": len(views)},
	})
}

// handleBoardSend serves CmdBoardSend (M937): a board write from outside a run
// — an SDK app or script posting, DMing, broadcasting, replying, or raising
// help. Mirrors the `board` tool's write semantics and fires the same
// board.posted notifier, so external mail wakes standing orders identically.
// correlation_id is optional; channel bridges and SDKs use it to keep mailbox
// wake events tied to the inbound message/run that caused the post.
func (s *Server) handleBoardSend(conn net.Conn, req Request) {
	st, ok := s.boardWriter()
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "the board is not available on this daemon"})
		return
	}
	text := stringArg(req.Args, "text")
	if text == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "board_send requires text"})
		return
	}
	from := stringArg(req.Args, "from")
	to := stringArg(req.Args, "to")
	topic := stringArg(req.Args, "topic")
	replyTo := stringArg(req.Args, "reply_to")
	corr := stringArg(req.Args, "correlation_id")
	help, _ := req.Args["help"].(bool)
	now := time.Now().UnixMilli()

	var m board.Message
	var err error
	switch {
	case replyTo != "":
		// A reply goes back to the asker on the original topic (the board tool's
		// op=reply semantics), so op=replies on the original id finds it.
		orig, found := st.Get(replyTo)
		if !found {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no message with id " + replyTo})
			return
		}
		m, err = st.Send(board.Message{Topic: orig.Topic, From: from, To: orig.From, ReplyTo: orig.ID, Text: text}, now)
	case help:
		m, err = st.HelpRequest(from, to, text, now)
	case to == board.Everyone:
		m, err = st.Broadcast(from, text, now)
	case to != "":
		if topic == "" {
			topic = "dm"
		}
		m, err = st.Send(board.Message{Topic: topic, From: from, To: to, Text: text}, now)
	default:
		if topic == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: "board_send requires a topic (for a post) or a to (for a DM / \"*\" broadcast)"})
			return
		}
		m, err = st.Post(topic, from, text, now)
	}
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if s.boardNotify != nil {
		s.boardNotify(m, corr)
	}
	res := map[string]any{"sent": boardMsgView(m)}
	if corr != "" {
		res["correlation_id"] = corr
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

// handleBoardInbox serves CmdBoardInbox (M937): what is waiting for a named
// agent/app — addressed messages plus broadcasts it didn't send, unanswered
// and unacked first. Read-only.
func (s *Server) handleBoardInbox(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	to := stringArg(req.Args, "to")
	if to == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "board_inbox requires to (whose inbox)"})
		return
	}
	all, _ := req.Args["all"].(bool)
	msgs := st.Inbox(to, boardLimitArg(req.Args), all)
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, boardMsgView(m))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"to": to, "waiting": views, "count": len(views)},
	})
}

// handleBoardAck serves CmdBoardAck (M937): mark a message read for one reader
// so it leaves that reader's unanswered inbox without a reply. A write — it
// requires the shared store like board_send.
func (s *Server) handleBoardAck(conn net.Conn, req Request) {
	st, ok := s.boardWriter()
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "the board is not available on this daemon"})
		return
	}
	id := stringArg(req.Args, "id")
	by := stringArg(req.Args, "by")
	if id == "" || by == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "board_ack requires id and by"})
		return
	}
	_, found, err := st.Ack(id, by)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no message with id " + id})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult,
		Result: map[string]any{"acked": true, "id": id, "by": by}})
}

// handleBoardGet serves CmdBoardGet (M938): one message by id — how a watcher
// that learned an id from a board.posted event (which carries no text) fetches
// the body. Read-only.
func (s *Server) handleBoardGet(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "board_get requires id"})
		return
	}
	m, found := st.Get(id)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no message with id " + id})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"message": boardMsgView(m)}})
}

// handleBoardReplies serves CmdBoardReplies (M937): the answers to a message,
// oldest first — what the asker reads back. Read-only.
func (s *Server) handleBoardReplies(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "board_replies requires id"})
		return
	}
	msgs := st.Replies(id, boardLimitArg(req.Args))
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, boardMsgView(m))
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"id": id, "replies": views, "count": len(views)},
	})
}
