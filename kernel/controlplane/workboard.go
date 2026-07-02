// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
)

func workboardTaskView(t workboard.Task) map[string]any {
	b, _ := json.Marshal(t)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["comment_count"] = len(t.Comments)
	m["link_count"] = len(t.Links)
	m["attempt_count"] = len(t.Attempts)
	m["failed_attempt_count"] = workboard.FailedAttemptCount(t)
	if len(t.Criteria) > 0 {
		met := 0
		for _, c := range t.Criteria {
			if c.Met {
				met++
			}
		}
		m["criteria_count"] = len(t.Criteria)
		m["criteria_met"] = met
		m["gated"] = true
		m["proven"] = t.Proof != nil && t.Proof.Satisfied()
	}
	if decision := workboard.RetryDecisionFor(t, ""); decision.MaxAttempts > 0 {
		m["max_attempts"] = decision.MaxAttempts
		if decision.NextAttempt > 0 {
			m["next_attempt"] = decision.NextAttempt
		}
	}
	return m
}

func workboardDependencyStateViews(states []workboard.DependencyState) []map[string]any {
	out := make([]map[string]any, 0, len(states))
	for _, st := range states {
		row := map[string]any{
			"id":     st.ID,
			"status": string(st.Status),
		}
		if st.Title != "" {
			row["title"] = st.Title
		}
		if st.Missing {
			row["missing"] = true
		}
		if st.CreatedMS > 0 {
			row["created_ms"] = st.CreatedMS
		}
		out = append(out, row)
	}
	return out
}

func workboardDependencySummary(states []workboard.DependencyState) string {
	parts := make([]string, 0, len(states))
	for _, st := range states {
		status := string(st.Status)
		if st.Missing {
			status = "missing"
		}
		if st.Title != "" {
			parts = append(parts, fmt.Sprintf("%s(%s:%s)", st.ID, st.Title, status))
		} else {
			parts = append(parts, fmt.Sprintf("%s(%s)", st.ID, status))
		}
	}
	return strings.Join(parts, ", ")
}

func (s *Server) handleWorkboardList(conn net.Conn, req Request) {
	var filter workboard.Filter
	if raw := stringArg(req.Args, "status"); raw != "" {
		st, err := workboard.ParseStatus(raw)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		filter.Status = st
	}
	filter.Tenant = stringArg(req.Args, "tenant")
	filter.Assignee = stringArg(req.Args, "assignee")
	filter.IncludeArchived, _ = req.Args["include_archived"].(bool)
	filter.Limit = intArg(req.Args["limit"], 100)
	tasks := s.k.Workboard().List(filter)
	out := make([]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, workboardTaskView(t))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tasks": out, "count": len(out)}})
}

func (s *Server) handleWorkboardLanes(conn net.Conn, req Request) {
	var filter workboard.Filter
	if raw := stringArg(req.Args, "status"); raw != "" {
		st, err := workboard.ParseStatus(raw)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		filter.Status = st
	}
	filter.Tenant = stringArg(req.Args, "tenant")
	filter.IncludeArchived, _ = req.Args["include_archived"].(bool)
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
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	if cleared, _ := req.Args["clear"].(bool); !cleared {
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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

func (s *Server) runWorkboardDispatch(corr string, p roster.Profile, task workboard.Task, intent, reason string) {
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	ctx = kernelruntime.WithWakeContext(ctx, kernelruntime.WakeContext{
		Source:         "workboard",
		Reason:         reason,
		TriggerSubject: "workboard." + task.ID,
	})
	if p.MaxCostMc > 0 {
		ctx = kernelruntime.WithMaxCost(ctx, p.MaxCostMc)
	}
	// Execution seat: refine HOW this task runs (model tier, tool tier, isolation
	// surface) on top of the assigned agent. Applied after WithAgentProfile so the
	// seat overrides where it sets an axis and the agent supplies the rest.
	// Isolation resolves seat-over-agent: the task's seat wins if it pins one,
	// otherwise the agent's own default execution profile applies.
	isoProfile := strings.TrimSpace(p.ExecutionProfile)
	isoSource := "agent"
	if seatID := strings.TrimSpace(task.Seat); seatID != "" && !strings.EqualFold(seatID, "default") {
		st, ok := s.k.Seats().Get(seatID)
		if !ok {
			// A seat that was removed or mistyped shouldn't vanish silently.
			_, _ = s.k.CommentWorkboardTask(corr, task.ID, "workboard", fmt.Sprintf("seat %q is unknown — running with agent defaults", seatID))
		} else {
			if len(st.ModelChain) > 0 {
				ctx = kernelruntime.WithModel(ctx, st.ModelChain[0])
				ctx = kernelruntime.WithModelChain(ctx, st.ModelChain)
			}
			if st.RestrictTools {
				ctx = kernelruntime.WithTools(ctx, st.Tools)
			}
			if st.ExecutionProfile != "" {
				isoProfile = st.ExecutionProfile
				isoSource = "seat " + st.ID
			}
		}
	}
	if isoProfile != "" {
		if nctx, _, perr := s.applyWardenExecutionProfile(ctx, isoProfile); perr != nil {
			// Degrade rather than fail: run with tool defaults and record why.
			_, _ = s.k.CommentWorkboardTask(corr, task.ID, "workboard", fmt.Sprintf("%s isolation %q unavailable (%v) — running with tool defaults", isoSource, isoProfile, perr))
		} else {
			ctx = nctx
		}
	}
	var (
		answer string
		err    error
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = s.k.RunWith(ctx, corr, intent)
	}
	if err != nil {
		failed, decision, failErr := s.k.FailWorkboardTask(corr, task.ID, p.Slug, "dispatch failed: "+err.Error())
		if failErr != nil {
			_, _ = s.k.BlockWorkboardTask(corr, task.ID, p.Slug, "dispatch failed: "+err.Error())
			publishWorkboardDispatch(s.k, corr, task, "failed", p.Slug, reason, "", err.Error())
			return
		}
		task = failed
		publishWorkboardDispatch(s.k, corr, task, decision.Action, p.Slug, reason, "", err.Error())
		if decision.Retry {
			claimed, claimErr := s.k.ClaimWorkboardTask(corr, task.ID, p.Slug, corr)
			if claimErr != nil {
				_, _ = s.k.BlockWorkboardTask(corr, task.ID, p.Slug, "retry claim failed: "+claimErr.Error())
				publishWorkboardDispatch(s.k, corr, task, "failed", p.Slug, reason, "", claimErr.Error())
				return
			}
			publishWorkboardDispatch(s.k, corr, claimed, "retrying", p.Slug, reason, "", "")
			s.runWorkboardDispatch(corr, p, claimed, intent, reason)
		}
		return
	}
	if current, found := s.k.Workboard().Get(task.ID); found && current.Status == workboard.StatusRunning && current.Claim != nil && current.Claim.RunID == corr {
		if len(current.Criteria) > 0 {
			// Proof loop: judge the answer against the task's acceptance criteria.
			// A satisfying proof drives the task to done; otherwise it parks in
			// review with the gap. Fall back to a plain review if proving errors.
			if proved, perr := s.k.ProveTask(ctx, corr, task.ID, answer); perr == nil {
				task = proved
			} else {
				task, _ = s.k.ReviewWorkboardTask(corr, task.ID, p.Slug, truncate(answer, 240))
			}
		} else {
			task, _ = s.k.ReviewWorkboardTask(corr, task.ID, p.Slug, truncate(answer, 240))
		}
	} else if found {
		task = current
	}
	publishWorkboardDispatch(s.k, corr, task, "completed", p.Slug, reason, truncate(answer, 300), "")
}

// applyWardenExecutionProfile resolves a warden-family execution profile id
// (local|warden|container) and layers its sandbox override onto ctx, returning
// the effective label. It mirrors the warden branch of the run handler
// (server.go handleRun) but returns errors as values so the async dispatch path
// can degrade gracefully. Remote backends (ssh/k8s/modal/daytona) are handled
// only by handleRun and are rejected here.
func (s *Server) applyWardenExecutionProfile(ctx context.Context, id string) (context.Context, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx, "", nil
	}
	if ok, reason := executionprofile.PolicyFromEnv().Allows(id); !ok {
		return ctx, "", fmt.Errorf("blocked by policy: %s", reason)
	}
	switch strings.ToLower(id) {
	case "ssh", "k8s", "modal", "daytona", "remote-agezt":
		return ctx, "", fmt.Errorf("seat isolation %q is only available on direct runs, not workboard dispatch", id)
	}
	p, ok := executionprofile.WardenProfileForRun(id)
	if !ok {
		return ctx, "", fmt.Errorf("%q is not a routable execution profile", id)
	}
	if p == warden.ProfileContainer && s.k.Warden().EffectiveProfile(p) != warden.ProfileContainer {
		return ctx, "", fmt.Errorf("%q requires an active container backend", id)
	}
	return warden.WithProfileOverride(ctx, p), string(p), nil
}

func buildWorkboardDispatchIntent(explicit string, task workboard.Task) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	var b strings.Builder
	b.WriteString("Workboard task dispatch.\n")
	b.WriteString("You are assigned a durable AGEZT workboard task. Use the workboard tool to heartbeat, comment, block, link artifacts/runs, and complete the task when it is actually done.\n")
	b.WriteString("Task ID: ")
	b.WriteString(task.ID)
	b.WriteString("\nTitle: ")
	b.WriteString(task.Title)
	b.WriteString("\nStatus: ")
	b.WriteString(string(task.Status))
	if task.Priority != 0 {
		b.WriteString("\nPriority: ")
		b.WriteString(fmt.Sprintf("%d", task.Priority))
	}
	if task.Tenant != "" {
		b.WriteString("\nTenant: ")
		b.WriteString(task.Tenant)
	}
	if task.Description != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(task.Description)
	}
	if len(task.Tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(task.Tags, ", "))
	}
	b.WriteString("\nExpected finish: call workboard {\"op\":\"complete\",\"id\":\"")
	b.WriteString(task.ID)
	b.WriteString("\"} only when complete; otherwise call workboard block/comment with the concrete reason or next step.")
	return b.String()
}

func publishWorkboardDispatch(k *kernelruntime.Kernel, corr string, task workboard.Task, phase, agent, reason, answer, errText string) {
	if k == nil || k.Bus() == nil {
		return
	}
	payload := map[string]any{
		"phase":          phase,
		"id":             task.ID,
		"title":          task.Title,
		"status":         string(task.Status),
		"agent":          agent,
		"reason":         reason,
		"correlation_id": corr,
	}
	if answer != "" {
		payload["answer"] = answer
	}
	if errText != "" {
		payload["error"] = errText
	}
	if task.Seat != "" {
		payload["seat"] = task.Seat
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "workboard." + task.ID,
		Kind:          event.KindWorkboardTaskDispatched,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func latestWorkboardRunID(task workboard.Task) string {
	if task.Claim != nil && strings.TrimSpace(task.Claim.RunID) != "" {
		return strings.TrimSpace(task.Claim.RunID)
	}
	var best string
	var bestMS int64
	for _, a := range task.Attempts {
		ts := a.StartedMS
		if a.FinishedMS > ts {
			ts = a.FinishedMS
		}
		if strings.TrimSpace(a.RunID) != "" && ts >= bestMS {
			bestMS = ts
			best = strings.TrimSpace(a.RunID)
		}
	}
	for _, l := range task.Links {
		if strings.EqualFold(l.Type, "run") && strings.TrimSpace(l.Target) != "" && l.CreatedMS >= bestMS {
			bestMS = l.CreatedMS
			best = strings.TrimSpace(l.Target)
		}
	}
	return best
}

func workboardWatchEvents(k *kernelruntime.Kernel, taskID, runID string, limit int) []map[string]any {
	if k == nil || k.Journal() == nil {
		return nil
	}
	subject := "workboard." + taskID
	var rows []map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject != subject && (runID == "" || e.CorrelationID != runID) {
			return nil
		}
		var payload any
		if len(e.Payload) > 0 {
			var m map[string]any
			if json.Unmarshal(e.Payload, &m) == nil {
				payload = m
			}
		}
		row := map[string]any{
			"seq":            e.Seq,
			"ts_unix_ms":     e.TSUnixMS,
			"kind":           string(e.Kind),
			"subject":        e.Subject,
			"correlation_id": e.CorrelationID,
		}
		if payload != nil {
			row["payload"] = payload
		}
		rows = append(rows, row)
		return nil
	})
	sort.SliceStable(rows, func(i, j int) bool {
		return intNumber(rows[i]["seq"]) < intNumber(rows[j]["seq"])
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows
}

func workboardWriteResp(s *Server, conn net.Conn, req Request, task workboard.Task, err error) {
	if err != nil {
		msg := err.Error()
		if errors.Is(err, workboard.ErrNotFound) {
			msg = "unknown workboard task: " + stringArg(req.Args, "id")
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: msg})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"task": workboardTaskView(task)}})
}

func retryPolicyFromArgs(args map[string]any) *workboard.RetryPolicy {
	if _, ok := args["max_attempts"]; !ok && stringArg(args, "escalate_to") == "" {
		return nil
	}
	return &workboard.RetryPolicy{
		MaxAttempts: intArgAllowZero(args["max_attempts"]),
		EscalateTo:  stringArg(args, "escalate_to"),
	}
}

func retryDecisionView(d workboard.RetryDecision) map[string]any {
	out := map[string]any{
		"action":        d.Action,
		"failure_count": d.FailureCount,
		"retry":         d.Retry,
		"exhausted":     d.Exhausted,
	}
	if d.MaxAttempts > 0 {
		out["max_attempts"] = d.MaxAttempts
	}
	if d.NextAttempt > 0 {
		out["next_attempt"] = d.NextAttempt
	}
	if d.EscalateTo != "" {
		out["escalate_to"] = d.EscalateTo
	}
	if d.Reason != "" {
		out["reason"] = d.Reason
	}
	return out
}

func workboardCorr(s *Server, req Request) string {
	if corr := stringArg(req.Args, "correlation_id"); corr != "" {
		return corr
	}
	return s.k.NewCorrelation()
}

func intArgAllowZero(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func workboardStringSliceArg(raw any) []string {
	switch xs := raw.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, raw := range xs {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.Split(s, ",")
		}
		return nil
	}
}

func workboardStatusList() string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		workboard.StatusTriage, workboard.StatusTodo, workboard.StatusReady, workboard.StatusRunning,
		workboard.StatusBlocked, workboard.StatusReview, workboard.StatusDone, workboard.StatusArchived)
}
