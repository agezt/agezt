// SPDX-License-Identifier: MIT
//
// Workboard control-plane handlers — read-only + lifecycle.
// handleWorkboardLanes + handleWorkboardShow (the read-only handlers)
// + handleWorkboardClaim + handleWorkboardHeartbeat +
// handleWorkboardComment + handleWorkboardBlock + handleWorkboardFail +
// handleWorkboardUnblock + handleWorkboardComplete + handleWorkboardProve +
// handleWorkboardSeat + handleWorkboardArchive (the lifecycle handlers).
// The link/policy/depend handlers live in workboard_handlers_link.go;
// the dispatch/watch handlers live in workboard_handlers_dispatch.go.
// Extracted from workboard_handlers.go during the Day-211 god-file split.
// Public API unchanged.
package controlplane

import (
	"context"
	"net"
	"sort"
	"strings"
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
	tasks := s.k.Workboard().List(filter)
	type lane struct {
		Assignee string
		Label    string
		Counts   map[string]int
		Tasks    []any
	}
	byAssignee := map[string]*lane{}
	for _, t := range tasks {
		key := strings.TrimSpace(t.Assignee)
		l := byAssignee[key]
		if l == nil {
			label := key
			if label == "" {
				label = "unassigned"
			}
			l = &lane{Assignee: key, Label: label, Counts: map[string]int{}}
			byAssignee[key] = l
		}
		l.Counts[string(t.Status)]++
		l.Tasks = append(l.Tasks, workboardTaskView(t))
	}
	keys := make([]string, 0, len(byAssignee))
	for key := range byAssignee {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i] == "" {
			return false
		}
		if keys[j] == "" {
			return true
		}
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		l := byAssignee[key]
		out = append(out, map[string]any{
			"assignee": l.Assignee,
			"label":    l.Label,
			"counts":   l.Counts,
			"tasks":    l.Tasks,
			"count":    len(l.Tasks),
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"lanes": out, "count": len(out), "task_count": len(tasks)}})
}

func (s *Server) handleWorkboardShow(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_show requires id"})
		return
	}
	task, found := s.k.Workboard().Get(id)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workboard task: " + id})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"task": workboardTaskView(task)}})
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
	task, created, err := s.k.CreateWorkboardTask(workboardCorr(s, req), workboard.CreateSpec{
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
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"task": workboardTaskView(task), "created": created}})
}

func (s *Server) handleWorkboardClaim(conn net.Conn, req Request) {
	task, err := s.k.ClaimWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "agent"), stringArg(req.Args, "run_id"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardHeartbeat(conn net.Conn, req Request) {
	task, err := s.k.HeartbeatWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "agent"), stringArg(req.Args, "run_id"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardComment(conn net.Conn, req Request) {
	task, err := s.k.CommentWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "author"), stringArg(req.Args, "body"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardBlock(conn net.Conn, req Request) {
	task, err := s.k.BlockWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"), stringArg(req.Args, "reason"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardFail(conn net.Conn, req Request) {
	task, decision, err := s.k.FailWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"), stringArg(req.Args, "reason"))
	if err != nil {
		workboardWriteResp(s, conn, req, task, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"task": workboardTaskView(task), "decision": retryDecisionView(decision)}})
}

func (s *Server) handleWorkboardUnblock(conn net.Conn, req Request) {
	task, err := s.k.UnblockWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardComplete(conn net.Conn, req Request) {
	task, err := s.k.CompleteWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"))
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardProve(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_prove requires id"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	task, err := s.k.ProveTask(ctx, workboardCorr(s, req), id, stringArg(req.Args, "answer"))
	workboardWriteResp(s, conn, req, task, err)
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
	task, err := s.k.Workboard().SetSeat(id, seatID, time.Now())
	workboardWriteResp(s, conn, req, task, err)
}

func (s *Server) handleWorkboardArchive(conn net.Conn, req Request) {
	task, err := s.k.ArchiveWorkboardTask(workboardCorr(s, req), stringArg(req.Args, "id"), stringArg(req.Args, "actor"))
	workboardWriteResp(s, conn, req, task, err)
}

