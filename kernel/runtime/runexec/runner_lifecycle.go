// SPDX-License-Identifier: MIT

// Runner lifecycle + maybe-helpers: CompleteAgentLifecycle + RunAssured + MaybeDistill + MaybeForge + MaybeShadowEval.
// Code extracted from runner.go during the Day-63 god-file split. Public API unchanged.
package runexec


import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"strings"
)


func (r *Runner) CompleteAgentLifecycle(ctx context.Context, corr string) {
	slug := r.k.AgentSlugFromCtx(ctx)
	if slug == "" {
		return
	}
	current, ok := r.k.Roster().Get(slug)
	if !ok || current.Retired {
		return
	}
	lifecycle := current.Lifecycle
	if shouldRetireAgentAfterComplete(lifecycle) {
		_, _ = r.k.SetProfileRetired(slug, true, "completed run "+corr)
		return
	}
	if strings.TrimSpace(lifecycle.Mode) != roster.LifecycleCycle && lifecycle.MaxCycles <= 0 {
		return
	}
	var completed, max int
	var advanced bool
	_, found, err := r.k.UpdateProfile(slug, func(p *roster.Profile) {
		// Idempotency: one logical run (correlation) advances the cycle
		// exactly once. RunAssured/RunWithRetry re-invoke RunWith
		// under the SAME corr (re-running until the work verifies
		// complete / a transient error clears); each inner success
		// calls here, so without this guard a single logical run would
		// double-count. The check sits inside the atomic UpdateProfile,
		// so it is race-free, and the marker is durable across
		// restarts. A correlation-less completion (corr=="") is never
		// guarded.
		if corr != "" && strings.TrimSpace(p.Lifecycle.LastCompletedRun) == corr {
			completed = p.Lifecycle.CompletedCycles
			max = p.Lifecycle.MaxCycles
			return
		}
		if strings.TrimSpace(p.Lifecycle.Mode) == "" {
			p.Lifecycle.Mode = roster.LifecycleCycle
		}
		p.Lifecycle.CompletedCycles++
		if corr != "" {
			p.Lifecycle.LastCompletedRun = corr
		}
		resetCompletedCycleTasks(p.TaskList)
		completed = p.Lifecycle.CompletedCycles
		max = p.Lifecycle.MaxCycles
		advanced = true
	})
	if err != nil || !found || !advanced {
		return
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "roster." + slug,
		Kind:          event.KindRosterUpdated,
		Actor:         "roster",
		CorrelationID: corr,
		Payload: map[string]any{
			"slug":             slug,
			"action":           "lifecycle_cycle_completed",
			"completed_cycles": completed,
			"max_cycles":       max,
			"run":              corr,
		},
	})
	if max > 0 && completed >= max {
		_, _ = r.k.SetProfileRetired(slug, true, fmt.Sprintf("completed %d/%d cycles on run %s", completed, max, corr))
	}
}

// RunAssured is the "do-it-for-sure" loop (M651). Body migrated
// from kernel/runtime/runexec.go (Day 33). Same Resume-ticket
// ownership dance as before; the inner RunWith / RunWithRetry
// calls already route through the Runner (or *Kernel for
// RunWith, which still lives on the host — Day 23+ documented
// gap).
func (r *Runner) RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assure.Result, error) {
	ctx, owns := r.k.ClaimResumeTicket(ctx, corr, intent, resume.KindAssured, maxAttempts)
	res, err := assure.Until(ctx, intent, maxAttempts,
		func(ctx context.Context, _ int, task string) (string, error) {
			if pol, ok := r.k.AgentRetryPolicyFromCtx(ctx); ok && pol.MaxAttempts > 1 {
				return r.k.RunWithRetry(ctx, corr, task, pol)
			}
			return r.k.RunWith(ctx, corr, task)
		},
		func(ctx context.Context, task, answer string) (assure.Verdict, error) {
			return r.VerifyCompletion(ctx, corr, task, answer)
		},
	)
	if owns {
		r.k.FinalizeResumeTicket(corr, err)
	}
	return res.Answer, res, err
}

// MaybeDistill folds the run's journal and, if the run made at
// least MemoryDistillMinTools tool calls, runs one best-effort
// distillation pass over a compact transcript (Day 32). Best-
// effort: a failure is journaled as a memory distill error and
// swallowed — distillation must not turn a successful task into
// a failed one. Body moved from kernel/runtime/runexec.go.
func (r *Runner) MaybeDistill(ctx context.Context, corr, intent, answer string) {
	minTools := r.k.MemoryDistillMinTools()
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := r.k.FoldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := r.k.Memory().Distill(ctx, corr, r.k.Provider(), r.k.Model(), intent, transcript); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "memory.distill_failed",
			Kind:          event.KindMemoryWritten,
			Actor:         "memory",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "distill_failed", "error": err.Error()},
		})
	}
}

// MaybeForge proposes a DRAFT skill from the run's journal fold
// when the run made at least SkillForgeMinTools tool calls (Day 32).
// The operator promotes DRAFT skills — §5.1/§5.3. Same
// threshold-gated, never-fail-the-task contract as MaybeDistill.
func (r *Runner) MaybeForge(ctx context.Context, corr, intent, answer string) {
	minTools := r.k.SkillForgeMinTools()
	if minTools <= 0 {
		minTools = 4
	}
	toolCount, names := r.k.FoldRunTools(corr)
	if toolCount < minTools {
		return
	}
	transcript := buildTranscript(names, answer)
	if _, err := r.k.Forge().Propose(ctx, corr, r.k.Provider(), r.k.Model(), intent, transcript); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "skill.propose_failed",
			Kind:          event.KindSkillCreated,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"action": "propose_failed", "error": err.Error()},
		})
	}
}

// MaybeShadowEval judges the shadow skills relevant to a just-
// completed run (SPEC-05 §5.2, Day 32). Best-effort: a judge
// failure is journaled but never affects the run, which has
// already returned its answer.
func (r *Runner) MaybeShadowEval(ctx context.Context, corr, intent, answer string) {
	if err := r.k.Forge().ShadowEvaluate(ctx, corr, r.k.Provider(), r.k.Model(), intent, answer, shadowEvalLimit); err != nil {
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       "skill.shadow_eval_failed",
			Kind:          event.KindSkillShadowEval,
			Actor:         "forge",
			CorrelationID: corr,
			Payload:       map[string]any{"error": err.Error()},
		})
	}
}

// errStubOnly is the sentinel returned by stub entry points
// until a future slice fills the body. It is a distinctive
// error so callers fail loudly instead of silently getting a
// zero-value answer. Currently unused (Day 23 filled all
// planned entry points) but kept as a guard against silent
// regressions on future KernelAPI additions.
var errStubOnly = errors.New("runexec: Runner entry point not yet implemented")

// keep package-level references live so the imports in this
// file stay used after the body moves settle. Removing them
// would force a churn round for the next refactor slice.
var (
	_ = agent.RoleUser
	_ = assure.Result{}
)
