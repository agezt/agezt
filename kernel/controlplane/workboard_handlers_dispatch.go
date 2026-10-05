package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard control-plane handlers —
//             dispatch/watch. handleWorkboardDispatch (the long-running dispatch
//             handler) + handleWorkboardWatch (the streaming-watch handler).
//             Extracted from workboard_handlers.go during the Day-211 god-file
//             split. Public API unchanged.

import (
	"context"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/workboard"
	"net"
)

func (s *Server) handleWorkboardDispatch(conn net.Conn, req Request) {
	service := appworkboard.NewDispatch(s.k.Workboard(), s.k,
		func(ref string) (appworkboard.DispatchAgent, bool) {
			p, ok := s.k.Roster().Get(ref)
			if !ok {
				return appworkboard.DispatchAgent{}, false
			}
			direct := p.AllowsDirectCall()
			directError := ""
			if !direct {
				directError = managedSubagentDirectCallError(p, "dispatched")
			}
			return appworkboard.DispatchAgent{Slug: p.Slug, Retired: p.Retired, Enabled: p.Enabled, DirectAllowed: direct, DirectError: directError, Run: func(corr string, task workboard.Task, intent, reason string) {
				s.runWorkboardDispatch(corr, p, task, intent, reason)
			}}, true
		},
		func(corr string, task workboard.Task, phase, agent, reason, answer, errText string) {
			publishWorkboardDispatch(s.k, corr, task, phase, agent, reason, answer, errText)
		})
	out, err := service.Dispatch(context.Background(), appworkboard.DispatchInput{ID: stringArg(req.Args, "id"), Agent: stringArg(req.Args, "agent"), Reason: stringArg(req.Args, "reason"), Intent: stringArg(req.Args, "intent")})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardWatch(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.failMsg(conn, req, "workboard_watch requires id")
		return
	}
	limit := intArg(req.Args["limit"], 50)
	if limit > 200 {
		limit = 200
	}
	out, err := appworkboard.NewWatch(s.k.Workboard(), s.k.Journal()).Watch(context.Background(), appworkboard.WatchInput{ID: id, RunID: stringArg(req.Args, "run_id"), Limit: limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeWorkboardReadResult(s, conn, req, out)
}
