package controlplane

// Provenance: SPDX-License-Identifier: MIT Control-plane board handlers:
//             handleBoardRead + handleBoardHelp + handleBoardSend + handleBoardInbox
//             + handleBoardAck + handleBoardGet + handleBoardReplies +
//             registerBoardCommands. Extracted from board.go during the Day-202
//             god-file split. Public API unchanged.

import (
	"context"
	"encoding/json"
	appboard "github.com/agezt/agezt/kernel/app/board"
	"net"
)

func writeBoardResult(s *Server, conn net.Conn, req Request, out any) {
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
func (s *Server) handleBoardRead(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	topic, _, err := argString(req.Args, "topic")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cursor, _, err := argString(req.Args, "cursor")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, nil).Read(context.Background(), appboard.ReadInput{Topic: topic, Cursor: cursor, Limit: boardLimitArg(req.Args)})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardHelp(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, nil).Help(context.Background(), appboard.LimitInput{Limit: boardLimitArg(req.Args)})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardSend(conn net.Conn, req Request) {
	st, ok := s.boardWriter()
	if !ok {
		s.failMsg(conn, req, "the board is not available on this daemon")
		return
	}
	text := stringArg(req.Args, "text")
	if text == "" {
		s.failMsg(conn, req, "board_send requires text")
		return
	}
	from, to, topic, reply, corr := stringArg(req.Args, "from"), stringArg(req.Args, "to"), stringArg(req.Args, "topic"), stringArg(req.Args, "reply_to"), stringArg(req.Args, "correlation_id")
	help, _, err := argBool(req.Args, "help")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, s.boardNotify).Send(context.Background(), appboard.SendInput{Text: text, From: from, To: to, Topic: topic, ReplyTo: reply, CorrelationID: corr, Help: help})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardInbox(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	to := stringArg(req.Args, "to")
	if to == "" {
		s.failMsg(conn, req, "board_inbox requires to (whose inbox)")
		return
	}
	all, _, err := argBool(req.Args, "all")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, nil).Inbox(context.Background(), appboard.InboxInput{To: to, All: all, Limit: boardLimitArg(req.Args)})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardAck(conn net.Conn, req Request) {
	st, ok := s.boardWriter()
	if !ok {
		s.failMsg(conn, req, "the board is not available on this daemon")
		return
	}
	out, err := appboard.New(st, nil).Ack(context.Background(), appboard.AckInput{ID: stringArg(req.Args, "id"), By: stringArg(req.Args, "by")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardGet(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, nil).Get(context.Background(), appboard.GetInput{ID: stringArg(req.Args, "id")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}
func (s *Server) handleBoardReplies(conn net.Conn, req Request) {
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appboard.New(st, nil).Replies(context.Background(), appboard.RepliesInput{ID: stringArg(req.Args, "id"), Limit: boardLimitArg(req.Args)})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeBoardResult(s, conn, req, out)
}

// registerBoardCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerBoardCommands() {
	register(
		commandSpec{Cmd: CmdBoardRead, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBoardRead(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardHelp, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBoardHelp(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardSend, Handler: func(dc *DispatchCtx) { dc.S.handleBoardSend(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardInbox, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBoardInbox(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardAck, Handler: func(dc *DispatchCtx) { dc.S.handleBoardAck(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardReplies, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBoardReplies(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdBoardGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleBoardGet(dc.Conn, dc.Req) }},
	)
}
