package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard dispatch Server methods
//             (runWorkboardDispatch, applyWardenExecutionProfile). Extracted from
//             workboard_dispatch.go during Day 211 god-file refactor (#70). Public
//             API unchanged.

import (
	"context"
	"fmt"
	"strings"

	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
)

// The native bridge binds the actual profile/context APIs; business seat,
// retry settlement and proof/review orchestration live in app/workboard.
func (s *Server) runWorkboardDispatch(corr string, p roster.Profile, task workboard.Task, intent, reason string) {
	service := appworkboard.NewExecution(s.k, s.k.Seats(), s.k.Workboard(), workboardExecutionContext{s: s, profile: p},
		func(ctx context.Context, corr, intent string) (string, error) {
			if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
				return s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
			}
			return s.k.RunWith(ctx, corr, intent)
		}, func(corr string, task workboard.Task, phase, agent, reason, answer, errText string) {
			publishWorkboardDispatch(s.k, corr, task, phase, agent, reason, answer, errText)
		})
	_, _ = service.Run(context.Background(), appworkboard.ExecutionInput{CorrelationID: corr, Agent: appworkboard.ExecutionAgent{Slug: p.Slug, ExecutionProfile: p.ExecutionProfile}, Task: task, Intent: intent, Reason: reason})
}

type workboardExecutionContext struct {
	s       *Server
	profile roster.Profile
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
