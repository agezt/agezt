// SPDX-License-Identifier: MIT
//
// Workboard control-plane handlers — link/policy/depend.
// handleWorkboardLink + handleWorkboardPolicy + handleWorkboardDepend +
// handleWorkboardReclaim + handleWorkboardSweep.
// Extracted from workboard_handlers.go during the Day-211 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"time"

	"github.com/agezt/agezt/kernel/workboard"
)

func (s *Server) handleWorkboardLink(conn net.Conn, req Request) {
	task, err := s.k.LinkWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "type"), stringArg(req.Args, "target"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardPolicy(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_policy requires id"})
		return
	}
	var policy *workboard.RetryPolicy
	cleared, _, err := argBool(req.Args, "clear")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !cleared {
		if _, ok := req.Args["max_attempts"]; !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_policy requires max_attempts or clear"})
			return
		}
		maxAttempts := intArgAllowZero(req.Args["max_attempts"])
		if maxAttempts < 1 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_policy max_attempts must be positive"})
			return
		}
		policy = &workboard.RetryPolicy{MaxAttempts: maxAttempts, EscalateTo: stringArg(req.Args, "escalate_to")}
	}
	task, err := s.k.SetWorkboardRetryPolicy(workboardCorr(s, req), id, stringArg(req.Args, "actor"), policy)
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardDepend(conn net.Conn, req Request) {
	task, err := s.k.AddWorkboardDependency(workboardCorr(s, req), stringArg(req.Args, "id"), firstNonEmpty(stringArg(req.Args, "depends_on"), stringArg(req.Args, "on")))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardReclaim(conn net.Conn, req Request) {
	staleAfterMS := intArg(req.Args["stale_after_ms"], 10*60*1000)
	task, err := s.k.ReclaimStaleWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"), time.Duration(staleAfterMS)*time.Millisecond)
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardSweep(conn net.Conn, req Request) {
	staleAfterMS := intArg(req.Args["stale_after_ms"], 10*60*1000)
	limit := intArg(req.Args["limit"], 100)
	if limit > 1000 {
		limit = 1000
	}
	tasks, err := s.k.SweepStaleWorkboardClaims(workboardCorr(s, req), firstNonEmpty(stringArg(req.Args, "actor"), "workboard-sweeper"), time.Duration(staleAfterMS)*time.Millisecond, limit)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, workboardTaskView(t))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tasks": out, "reclaimed_count": len(out), "stale_after_ms": staleAfterMS}})
}

