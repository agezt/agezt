// SPDX-License-Identifier: MIT

// Workboard dispatch + watch internals: runWorkboardDispatch, applyWardenExecutionProfile, buildWorkboardDispatchIntent, publishWorkboardDispatch, latestWorkboardRunID, workboardWatchEvents, workboardWriteResp, retryPolicyFromArgs, retryDecisionView, workboardCorr, intArgAllowZero, workboardStringSliceArg, registerWorkboardCommands.
// Code extracted from workboard.go during the Day-48 god-file split. Public API unchanged.
package controlplane


import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
)


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

// registerWorkboardCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerWorkboardCommands() {
	register(
		commandSpec{Cmd: CmdWorkboardList, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardLanes, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardLanes(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardShow, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardCreate, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardCreate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardClaim, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardClaim(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardHeartbeat, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardHeartbeat(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardComment, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardComment(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardBlock, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardBlock(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardFail, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardFail(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardUnblock, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardUnblock(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardComplete, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardComplete(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardProve, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardProve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardSeat, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardSeat(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardArchive, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardArchive(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardLink, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardLink(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardPolicy, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardPolicy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardDepend, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardDepend(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardReclaim, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardReclaim(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardSweep, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardSweep(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardDispatch, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardDispatch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkboardWatch, Handler: func(dc *DispatchCtx) { dc.S.handleWorkboardWatch(dc.Conn, dc.Req) }},
	)
}
