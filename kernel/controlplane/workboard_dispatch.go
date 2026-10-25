package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard dispatch Server methods
//             (runWorkboardDispatch, applyWardenExecutionProfile). Extracted from
//             workboard_dispatch.go during Day 211 god-file refactor (#70). Public
//             API unchanged.

import (
	"context"
	"fmt"
	"strings"

	appruns "github.com/agezt/agezt/kernel/app/runs"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/executionprofile"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// The native bridge binds the actual profile/context APIs; business seat,
// retry settlement and proof/review orchestration live in app/workboard.
func (s *Server) runWorkboardDispatch(corr string, p agentapi.Profile, task workboardapi.Task, intent, reason string) {
	service := appworkboard.NewExecution(s.k, s.k.Seats(), s.k.Workboard(), workboardExecutionContext{s: s, profile: p},
		func(ctx context.Context, corr, intent string) (string, error) {
			if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
				return s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
			}
			return s.k.RunWith(ctx, corr, intent)
		}, func(corr string, task workboardapi.Task, phase, agent, reason, answer, errText string) {
			publishWorkboardDispatch(s.k, corr, task, phase, agent, reason, answer, errText)
		})
	_, _ = service.Run(context.Background(), appworkboard.ExecutionInput{CorrelationID: corr, Agent: appworkboard.ExecutionAgent{Slug: p.Slug, ExecutionProfile: p.ExecutionProfile}, Task: task, Intent: intent, Reason: reason})
}

type workboardExecutionContext struct {
	s       *Server
	profile agentapi.Profile
}

func (b workboardExecutionContext) Agent(ctx context.Context, reason, taskID string) context.Context {
	ctx = kernelruntime.WithAgentProfile(ctx, b.profile)
	ctx = kernelruntime.WithWakeContext(ctx, kernelruntime.WakeContext{Source: "workboard", Reason: reason, TriggerSubject: "workboard." + taskID})
	if b.profile.MaxCostMc > 0 {
		ctx = kernelruntime.WithMaxCost(ctx, b.profile.MaxCostMc)
	}
	return ctx
}
func (b workboardExecutionContext) Models(ctx context.Context, chain []string) context.Context {
	ctx = kernelruntime.WithModel(ctx, chain[0])
	return kernelruntime.WithModelChain(ctx, chain)
}
func (b workboardExecutionContext) Tools(ctx context.Context, tools []string) context.Context {
	return kernelruntime.WithTools(ctx, tools)
}
func (b workboardExecutionContext) Isolation(ctx context.Context, id string) (context.Context, string, error) {
	return b.s.applyWardenExecutionProfile(ctx, id)
}

// applyWardenExecutionProfile resolves a warden-family execution profile id
// (local|warden|container) and layers its sandbox override onto ctx, returning
// the effective label. The resolution itself belongs to the application layer
// (app/runs.WardenProfile), so the dispatched seat path and the direct run
// path cannot drift apart. Remote backends (ssh/k8s/modal/daytona/
// remote-agezt) are handled only by the direct run path and are rejected
// here; the list of those backends is the executionprofile package's, so a
// new one cannot be forgotten here.
func (s *Server) applyWardenExecutionProfile(ctx context.Context, id string) (context.Context, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx, "", nil
	}
	if ok, reason := executionprofile.PolicyFromEnv().Allows(id); !ok {
		return ctx, "", fmt.Errorf("blocked by policy: %s", reason)
	}
	if executionprofile.IsRemoteBackend(id) {
		return ctx, "", fmt.Errorf("seat isolation %q is only available on direct runs, not workboard dispatch", id)
	}
	return appruns.WardenProfile(ctx, s.k, id)
}
