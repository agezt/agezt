package controlplane

// Provenance: SPDX-License-Identifier: MIT Control-plane board surface: the
//             boardRead* consts + the boardReader / boardWriter / boardLimitArg /
//             boardMsgView helpers. Extracted from board.go during the Day-202
//             god-file split. Public API unchanged.

import (
	"errors"

	"github.com/agezt/agezt/kernel/contract/boardapi"
)

// boardReadMaxLimit bounds CmdBoardRead.
const boardReadMaxLimit = 500

// errBoardUnavailable is the answer for a daemon with no board.
//
// The board's FILE is the daemon's business, so the control plane holds the
// CONTRACT's port and never opens a board of its own. A host that wired no
// board holds no port — and the reads and the writes answer the SAME thing,
// because "the board is not available on this daemon" is one fact and not two.
var errBoardUnavailable = errors.New("the board is not available on this daemon")

// boardReader returns the store for READ handlers: the daemon's shared
// instance — the ONLY instance.
//
// The board's FILE is the daemon's (W3.59): the control plane does not open a
// board of its own, so it may not name the module. The shared instance is the
// live one — every write goes through it and it saves the whole list — so
// reading through it sees exactly what a fresh read-only Open would have
// seen, without a second opener.
func (s *Server) boardReader() (boardapi.Store, error) {
	if s.boardStore == nil {
		return nil, errBoardUnavailable
	}
	return s.boardStore, nil
}

// boardWriter returns the store for WRITE handlers: ONLY the shared instance.
// A fresh Open here would race the `board` tool's instance — each holds the
// whole message list in memory and saves it whole, so the last writer would
// silently drop the other's message.
func (s *Server) boardWriter() (boardapi.Store, bool) {
	return s.boardStore, s.boardStore != nil
}
