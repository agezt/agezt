// SPDX-License-Identifier: MIT

// Workboard handlers (excluding List): handleWorkboardLanes/Show/Create/Claim/Heartbeat/Comment/Block/Fail/Unblock/Complete/Prove/Seat/Archive/Link/Policy/Depend/Reclaim/Sweep/Dispatch/Watch.
// Code extracted from workboard.go during the Day-48 god-file split. Public API unchanged.
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

func (s *Server) handleWorkboardDispatch(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_dispatch requires id"})
		return
	}
	task, found := s.k.Workboard().Get(id)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workboard task: " + id})
		return
	}
	blocked, err := s.k.Workboard().BlockingDependencies(task.ID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if len(blocked) > 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard task blocked by dependencies: " + workboardDependencySummary(blocked)})
		return
	}
	agentRef := firstNonEmpty(stringArg(req.Args, "agent"), task.Assignee)
	if agentRef == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_dispatch requires --agent or a task assignee"})
		return
	}
	p, ok := s.k.Roster().Get(agentRef)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + agentRef})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "dispatched")})
		return
	}

	corr := s.k.NewCorrelation()
	claimed, err := s.k.ClaimWorkboardTask(corr, task.ID, p.Slug, corr)
	if err != nil {
		workboardWriteResp(s, conn, req, claimed, err)
		return
	}
	if linked, err := s.k.LinkWorkboardTask(corr, task.ID, "run", corr); err != nil {
		workboardWriteResp(s, conn, req, linked, err)
		return
	} else {
		claimed = linked
	}
	reason := firstNonEmpty(stringArg(req.Args, "reason"), "workboard dispatch")
	intent := buildWorkboardDispatchIntent(stringArg(req.Args, "intent"), claimed)
	publishWorkboardDispatch(s.k, corr, claimed, "requested", p.Slug, reason, "", "")
	go s.runWorkboardDispatch(corr, p, claimed, intent, reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"task":           workboardTaskView(claimed),
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleWorkboardWatch(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "workboard_watch requires id"})
		return
	}
	task, found := s.k.Workboard().Get(id)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown workboard task: " + id})
		return
	}
	runID := firstNonEmpty(stringArg(req.Args, "run_id"), latestWorkboardRunID(task))
	limit := intArg(req.Args["limit"], 50)
	if limit > 200 {
		limit = 200
	}
	events := workboardWatchEvents(s.k, task.ID, runID, limit)
	blocked, _ := s.k.Workboard().BlockingDependencies(task.ID)
	res := map[string]any{"task": workboardTaskView(task), "events": events, "count": len(events), "blocked_dependencies": workboardDependencyStateViews(blocked)}
	if runID != "" {
		res["run_id"] = runID
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}
