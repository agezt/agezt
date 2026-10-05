package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard control-plane handlers —
//             read-only + lifecycle. handleWorkboardLanes + handleWorkboardShow (the
//             read-only handlers) + handleWorkboardClaim + handleWorkboardHeartbeat
//             + handleWorkboardComment + handleWorkboardBlock + handleWorkboardFail
//             + handleWorkboardUnblock + handleWorkboardComplete +
//             handleWorkboardProve + handleWorkboardSeat + handleWorkboardArchive
//             (the lifecycle handlers). The link/policy/depend handlers live in
//             workboard_handlers_link.go; the dispatch/watch handlers live in
//             workboard_handlers_dispatch.go. Extracted from workboard_handlers.go
//             during the Day-211 god-file split. Public API unchanged.

import (
	"context"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/workboard"
)

func (s *Server) handleWorkboardLanes(conn net.Conn, req Request) {
	var filter workboard.Filter
	if raw := stringArg(req.Args, "status"); raw != "" {
		st, err := workboard.ParseStatus(raw)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		filter.Status = st
	}
	filter.Tenant = stringArg(req.Args, "tenant")
	if v, _, err := argBool(req.Args, "include_archived"); err != nil {
		s.fail(conn, req, err)
		return
	} else {
		filter.IncludeArchived = v
	}
	filter.Limit = intArg(req.Args["limit"], 500)
	out, err := appworkboard.New(s.k.Workboard()).Lanes(context.Background(), appworkboard.ListInput{Status: filter.Status, Tenant: filter.Tenant, IncludeArchived: filter.IncludeArchived, Limit: filter.Limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeWorkboardReadResult(s, conn, req, out)
}
func (s *Server) handleWorkboardShow(conn net.Conn, req Request) {
	out, err := appworkboard.New(s.k.Workboard()).Show(context.Background(), appworkboard.ShowInput{ID: stringArg(req.Args, "id")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeWorkboardReadResult(s, conn, req, out)
}

func (s *Server) handleWorkboardCreate(conn net.Conn, req Request) {
	title := stringArg(req.Args, "title")
	if title == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_create requires title"})
		return
	}
	var status workboard.Status
	if raw := stringArg(req.Args, "status"); raw != "" {
		st, err := workboard.ParseStatus(raw)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		status = st
	}
	seatID := stringArg(req.Args, "seat")
	if !s.k.Seats().Valid(seatID) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown execution seat: " + seatID})
		return
	}
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Create(context.Background(), appworkboard.CreateInput{CorrelationID: workboardCorr(s, req), Spec: workboard.CreateSpec{
		Title:              title,
		Description:        stringArg(req.Args, "description"),
		Status:             status,
		Priority:           intArgAllowZero(req.Args["priority"]),
		Tenant:             stringArg(req.Args, "tenant"),
		Assignee:           stringArg(req.Args, "assignee"),
		Owner:              stringArg(req.Args, "owner"),
		IdempotencyKey:     stringArg(req.Args, "idempotency_key"),
		Tags:               workboardStringSliceArg(req.Args["tags"]),
		Artifacts:          workboardStringSliceArg(req.Args["artifacts"]),
		AcceptanceCriteria: workboardStringSliceArg(req.Args["criteria"]),
		Seat:               seatID,
		RetryPolicy:        retryPolicyFromArgs(req.Args),
	}})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardClaim(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Claim(context.Background(), appworkboard.ClaimInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Agent: stringArg(req.Args, "agent"), RunID: stringArg(req.Args, "run_id")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardHeartbeat(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Heartbeat(context.Background(), appworkboard.ClaimInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Agent: stringArg(req.Args, "agent"), RunID: stringArg(req.Args, "run_id")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardComment(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Comment(context.Background(), appworkboard.CommentInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Author: stringArg(req.Args, "author"), Body: stringArg(req.Args, "body")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardBlock(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Block(context.Background(), appworkboard.ReasonInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor"), Reason: stringArg(req.Args, "reason")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardFail(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Fail(context.Background(), appworkboard.ReasonInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor"), Reason: stringArg(req.Args, "reason")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardUnblock(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Unblock(context.Background(), appworkboard.ReasonInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
func (s *Server) handleWorkboardComplete(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Complete(context.Background(), appworkboard.ReasonInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor")})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardProve(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_prove requires id"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Prove(ctx, appworkboard.ProveInput{CorrelationID: workboardCorr(s, req), ID: id, Answer: stringArg(req.Args, "answer")})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardSeat(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_seat requires id"})
		return
	}
	seatID := stringArg(req.Args, "seat")
	if !s.k.Seats().Valid(seatID) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown execution seat: " + seatID})
		return
	}
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Seat(context.Background(), appworkboard.SeatInput{ID: id, Seat: seatID})
	writeWorkboardAppResult(s, conn, req, out, err)
}

func (s *Server) handleWorkboardArchive(conn net.Conn, req Request) {
	out, err := appworkboard.NewLifecycle(s.k, s.k.Workboard()).Archive(context.Background(), appworkboard.ReasonInput{CorrelationID: workboardCorr(s, req), ID: stringArg(req.Args, "id"), Actor: stringArg(req.Args, "actor")})
	writeWorkboardAppResult(s, conn, req, out, err)
}
