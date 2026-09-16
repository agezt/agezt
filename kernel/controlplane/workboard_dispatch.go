// SPDX-License-Identifier: MIT
//
// Workboard dispatch Server methods (runWorkboardDispatch, applyWardenExecutionProfile).
// Extracted from workboard_dispatch.go during Day 211 god-file refactor (#70).
// Public API unchanged.
package controlplane

import (
	"context"
	"fmt"
	"strings"

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
