// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"path/filepath"

	"github.com/agezt/agezt/kernel/board"
)

// boardReadDefaultLimit / boardReadMaxLimit bound CmdBoardRead.
const (
	boardReadDefaultLimit = 50
	boardReadMaxLimit     = 500
)

// handleBoardRead serves CmdBoardRead: a read-only view of the shared inter-agent
// message board so the Web UI can show agents talking to each other. The board
// is the `board` tool's store (kernel/board) under <baseDir>/board; we Open it
// fresh per request — writes are atomic, so a fresh Open sees the latest
// committed state without sharing the tool's in-process instance.
func (s *Server) handleBoardRead(conn net.Conn, req Request) {
	st, err := board.Open(filepath.Join(s.baseDir, "board"))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	limit := boardReadDefaultLimit
	if raw, ok := req.Args["limit"]; ok {
		if v, ok := raw.(float64); ok {
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > boardReadMaxLimit {
		limit = boardReadMaxLimit
	}
	topic, _ := req.Args["topic"].(string)

	msgs := st.Read(topic, limit)
	views := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		v := map[string]any{"topic": m.Topic, "text": m.Text, "ts_unix_ms": m.TSMS}
		if m.From != "" {
			v["from"] = m.From
		}
		// Addressed messaging (M788/M791): the console threads DMs and replies.
		if m.ID != "" {
			v["id"] = m.ID
		}
		if m.To != "" {
			v["to"] = m.To
		}
		if m.ReplyTo != "" {
			v["reply_to"] = m.ReplyTo
		}
		views = append(views, v)
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"messages": views, "topics": st.Topics(), "count": len(views)},
	})
}
