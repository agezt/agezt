package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard control-plane handlers —
//             link/policy/depend. handleWorkboardLink + handleWorkboardPolicy +
//             handleWorkboardDepend + handleWorkboardReclaim + handleWorkboardSweep.
//             Extracted from workboard_handlers.go during the Day-211 god-file
//             split. Public API unchanged.

import (
	"context"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"net"

	"github.com/agezt/agezt/kernel/workboard"
)

func (s *Server) handleWorkboardLink(conn net.Conn, req Request) {
	out, err := appworkboard.NewRelations(s.k).Link(context.Background(), appworkboard.LinkInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Type: stringArg(req.Args, "type"), Target: stringArg(req.Args, "target")})
	writeWorkboardAppResult(s, conn, req, out, err)
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
	out, err := appworkboard.NewRelations(s.k).Policy(context.Background(), appworkboard.PolicyInput{CorrelationID: workboardCorr(s, req), ID: id, Actor: stringArg(req.Args, "actor"), Policy: policy})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardDepend(conn net.Conn, req Request) {
	out, err := appworkboard.NewRelations(s.k).Depend(context.Background(), appworkboard.DependInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), DependsOn: firstNonEmpty(stringArg(req.Args, "depends_on"), stringArg(req.Args, "on"))})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardReclaim(conn net.Conn, req Request) {
	staleAfterMS := intArg(req.Args["stale_after_ms"], 10*60*1000)
	out, err := appworkboard.NewRelations(s.k).Reclaim(context.Background(), appworkboard.ReclaimInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor"), StaleAfterMS: staleAfterMS})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardSweep(conn net.Conn, req Request) {
	staleAfterMS := intArg(req.Args["stale_after_ms"], 10*60*1000)
	limit := intArg(req.Args["limit"], 100)
	if limit > 1000 {
		limit = 1000
	}
	out, err := appworkboard.NewRelations(s.k).Sweep(context.Background(), appworkboard.SweepInput{CorrelationID: workboardCorr(s, req), Actor: firstNonEmpty(stringArg(req.Args, "actor"), "workboard-sweeper"), StaleAfterMS: staleAfterMS, Limit: limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeWorkboardReadResult(s, conn, req, out)
}
