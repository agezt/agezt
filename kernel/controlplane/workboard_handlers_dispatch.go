// SPDX-License-Identifier: MIT
//
// Workboard control-plane handlers — dispatch/watch.
// handleWorkboardDispatch (the long-running dispatch handler) +
// handleWorkboardWatch (the streaming-watch handler).
// Extracted from workboard_handlers.go during the Day-211 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
)

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
