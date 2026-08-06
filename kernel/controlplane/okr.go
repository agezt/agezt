// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/agezt/agezt/kernel/okr"
)

func okrObjectiveView(s *Server, o okr.Objective) map[string]any {
	b, _ := json.Marshal(o)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	pr := s.k.Rollup(o)
	pb, _ := json.Marshal(pr)
	var pm map[string]any
	_ = json.Unmarshal(pb, &pm)
	m["progress"] = pm
	m["percent"] = pr.Percent
	m["achieved"] = pr.Achieved
	m["key_result_count"] = len(o.KeyResults)
	return m
}

func (s *Server) handleOKRList(conn net.Conn, req Request) {
	var f okr.Filter
	if raw := stringArg(req.Args, "status"); raw != "" {
		f.Status = okr.Status(raw)
	}
	f.Tenant = stringArg(req.Args, "tenant")
	if v, _, err := argBool(req.Args, "include_archived"); err != nil {
		s.fail(conn, req, err)
		return
	} else {
		f.IncludeArchived = v
	}
	f.Limit = intArg(req.Args["limit"], 200)
	objs := s.k.OKR().List(f)
	out := make([]any, 0, len(objs))
	for _, o := range objs {
		out = append(out, okrObjectiveView(s, o))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"objectives": out, "count": len(out)}})
}

func (s *Server) handleOKRShow(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "okr_show requires id"})
		return
	}
	o, ok := s.k.OKR().Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown objective: " + id})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"objective": okrObjectiveView(s, o)}})
}

func (s *Server) handleOKRCreate(conn net.Conn, req Request) {
	title := stringArg(req.Args, "title")
	if title == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "okr_create requires title"})
		return
	}
	o, err := s.k.CreateObjective(workboardCorr(s, req), okr.CreateSpec{
		Title:       title,
		Description: stringArg(req.Args, "description"),
		Owner:       stringArg(req.Args, "owner"),
		Tenant:      stringArg(req.Args, "tenant"),
	})
	okrWriteResp(s, conn, req, o, err)
}

func (s *Server) handleOKRKeyResult(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	title := stringArg(req.Args, "title")
	if id == "" || title == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "okr_keyresult requires id and title"})
		return
	}
	o, err := s.k.AddObjectiveKeyResult(workboardCorr(s, req), id, title, intArgAllowZero(req.Args["target"]))
	okrWriteResp(s, conn, req, o, err)
}

func (s *Server) handleOKRLink(conn net.Conn, req Request) {
	o, err := s.k.LinkObjectiveTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "key_result"), stringArg(req.Args, "task"))
	okrWriteResp(s, conn, req, o, err)
}

func (s *Server) handleOKRUnlink(conn net.Conn, req Request) {
	o, err := s.k.UnlinkObjectiveTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "key_result"), stringArg(req.Args, "task"))
	okrWriteResp(s, conn, req, o, err)
}

func (s *Server) handleOKRArchive(conn net.Conn, req Request) {
	o, err := s.k.ArchiveObjective(workboardCorr(s, req), stringArg(req.Args, "id"))
	okrWriteResp(s, conn, req, o, err)
}

func okrWriteResp(s *Server, conn net.Conn, req Request, o okr.Objective, err error) {
	if err != nil {
		msg := err.Error()
		if errors.Is(err, okr.ErrNotFound) {
			msg = "unknown objective: " + stringArg(req.Args, "id")
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: msg})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"objective": okrObjectiveView(s, o)}})
}

// registerOKRCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerOKRCommands() {
	register(
		commandSpec{Cmd: CmdOKRList, Handler: func(dc *DispatchCtx) { dc.S.handleOKRList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRShow, Handler: func(dc *DispatchCtx) { dc.S.handleOKRShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRCreate, Handler: func(dc *DispatchCtx) { dc.S.handleOKRCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRKeyResult, Handler: func(dc *DispatchCtx) { dc.S.handleOKRKeyResult(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRLink, Handler: func(dc *DispatchCtx) { dc.S.handleOKRLink(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRUnlink, Handler: func(dc *DispatchCtx) { dc.S.handleOKRUnlink(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdOKRArchive, Handler: func(dc *DispatchCtx) { dc.S.handleOKRArchive(dc.Conn, dc.Req) }},
	)
}
