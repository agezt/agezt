// SPDX-License-Identifier: MIT
//
// Control-plane board surface: the boardRead* consts + the boardReader /
// boardWriter / boardLimitArg / boardMsgView helpers.
// Extracted from board.go during the Day-202 god-file split.
// Public API unchanged.
package controlplane

import (
	"path/filepath"

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
