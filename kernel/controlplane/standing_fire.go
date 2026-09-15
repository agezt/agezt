// SPDX-License-Identifier: MIT

// controlplane/standing: fire-callback machinery (SetStandingFire +
// handleStandingFire + registerStandingCommands).
// Split from standing.go during Day 211 god-file refactor (#46).
// Public API unchanged.
package controlplane

import (
	"net"
)

func (s *Server) SetStandingFire(fn func(id string) bool) { s.standingFire = fn }

// handleStandingFire triggers one standing order now (M765) — the sibling of
// schedule "run now" and pulse "beat now". It launches the order's run regardless of
// its cron/event triggers (useful to test an order or run it on demand). Returns as
// soon as the run is dispatched; the result lands in the journal / Runs view.
func (s *Server) handleStandingFire(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if s.standingFire == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "standing-order firing is not available on this daemon"})
		return
	}
	o, ok := s.k.Standing().Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fired": false, "id": id}})
		return
	}
	if err := s.validateStandingAgent(o.Agent); err != nil {
		s.fail(conn, req, err)
		return
	}
	fired := s.standingFire(id)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fired": fired, "id": id}})
}

// registerStandingCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerStandingCommands() {
	register(
		commandSpec{Cmd: CmdStandingList, Handler: func(dc *DispatchCtx) { dc.S.handleStandingList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingAdd, Handler: func(dc *DispatchCtx) { dc.S.handleStandingAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingEdit, Handler: func(dc *DispatchCtx) { dc.S.handleStandingEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleStandingSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingRemove, Handler: func(dc *DispatchCtx) { dc.S.handleStandingRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingFire, Handler: func(dc *DispatchCtx) { dc.S.handleStandingFire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingWhy, Handler: func(dc *DispatchCtx) { dc.S.handleStandingWhy(dc.Conn, dc.Req) }},
	)
}
