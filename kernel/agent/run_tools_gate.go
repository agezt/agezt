// SPDX-License-Identifier: MIT

package agent

// Provenance: Agent run-tools: gateToolCalls (per-call policy + capability
//             matching). Code extracted from run_tools.go during the Day-127
//             god-file split. Public API unchanged.

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolaudit"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

func (s *runState) gateToolCalls(ctx context.Context, calls []ToolCall, iter int) ([]*toolJob, error) {
	jobs := make([]*toolJob, 0, len(calls))
	memoPending := map[string]*toolJob{}
	for _, tc := range calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		job := &toolJob{tc: tc}
		jobs = append(jobs, job)

		resolved := toolpipeline.Resolve(tc, func(name string) (toolapi.Tool, bool) {
			tool, ok := s.cfg.Tools[name]
			return tool, ok
		})
		if !resolved.Found {
			job.result = Result{
				Output:  fmt.Sprintf("tool %q is not available", tc.Name),
				IsError: true,
			}
			continue
		}
		if err := resolved.InputError; err != nil {
			job.result = Result{
				Output:  "tool call rejected by schema: " + err.Error(),
				IsError: true,
			}
			continue
		}
		tool, def := resolved.Tool, resolved.Definition

		// Loop guard (M116): if the model has already invoked this EXACT
		// (tool, input) the cap number of times in this run, refuse to run it
		// again — re-executing a stuck/failing call produces the same result and
		// wastes work (and money). Feed back a clear nudge so the model changes
		// approach instead of looping to MaxIter. Identical-input only, so a
		// legitimate re-call with different args is unaffected. A negative cap
		// disables the guard.
		if s.cfg.MaxIdenticalToolCalls > 0 {
			callKey := tc.Name + "\x00" + string(tc.Input)
			s.callCounts[callKey]++
			if s.callCounts[callKey] > s.cfg.MaxIdenticalToolCalls {
				job.result = Result{
					Output:  fmt.Sprintf("loop guard: %q was already called with this exact input %d times in this run; the result will not change. Use different input or stop and give your answer.", tc.Name, s.cfg.MaxIdenticalToolCalls),
					IsError: true,
				}
				continue
			}
		}

		// Policy gate (Edict). Publishes a policy.decision event even when no
		// Policy is configured, so the journal makes the gating posture explicit.
		// A Deny verdict short-circuits the invocation and synthesises a tool
		// result the model sees.
		policyCtx := ctx
		if s.cfg.Policy != nil {
			// The loop owns the causal window; the shared phase owns resolved metadata.
			scopedTaint := s.untrustedTaint
			scopedTaint.DirectiveLike = s.directiveActive(iter)
			policyCtx = WithUntrustedObservationTaint(policyCtx, scopedTaint)
		}
		decision, err := toolpipeline.Decide(policyCtx, tc, def,
			func(policyCtx context.Context, call ToolCall) PolicyVerdict {
				if s.cfg.Policy == nil {
					return PolicyVerdict{Allow: true, Capability: call.Name, Reason: "no policy configured"}
				}
				return s.cfg.Policy(policyCtx, call)
			},
			func(call ToolCall, verdict PolicyVerdict) error {
				_, err := s.publish(event.KindPolicyDecision, "policy", policyDecisionPayload(call, verdict))
				return err
			})
		if err != nil {
			return nil, fmt.Errorf("agent: publish policy.decision: %w", err)
		}
		verdict := decision.Verdict

		if !verdict.Allow {
			// Count the refusal so a tool that's hard-denied (never allowed) or
			// repeatedly refused is dropped from later iterations (M605).
			if verdict.HardDenied {
				s.toolDenials[tc.Name] = maxToolDenials
			} else {
				s.toolDenials[tc.Name]++
			}
			job.result = Result{
				Output:  "tool call denied by policy: " + verdict.Reason,
				IsError: true,
			}
			continue // skip tool.invoked when the call never runs
		}
		if s.cfg.ToolMemo != nil && (verdict.EffectClass == string(EffectReadOnly) || def.Effect.Class == EffectReadOnly) {
			if cached, ok := s.cfg.ToolMemo.Get(tc.Name, tc.Input); ok {
				job.result = cached
				job.memoHit = true
				continue
			}
			job.memoEligible = true
			key := memoKey(tc.Name, tc.Input)
			if source, ok := memoPending[key]; ok {
				job.memoSource = source
				job.memoHit = true
				continue
			}
			memoPending[key] = job
		}
		if err := toolpipeline.Announce(tc, func(kind event.Kind, payload map[string]any) error {
			_, err := s.publish(kind, "tool", payload)
			return err
		}); err != nil {
			return nil, fmt.Errorf("agent: publish tool.invoked: %w", err)
		}
		job.tool = tool
	}
	return jobs, nil
}

// policyDecisionPayload forwards the unchanged loop journal representation.
func policyDecisionPayload(tc ToolCall, v PolicyVerdict) map[string]any {
	return toolaudit.PolicyDecisionPayload(tc, v)
}

// executeToolJobs is phase 2: run every gated-allowed job. A single call (the
// common case) or disabled parallelism runs inline on the caller's goroutine;
// both paths use the shared invocation panic firewall. A multi-call turn fans out up to
// MaxParallelTools at a time, each worker carrying its OWN recover because Run's
// firewall only guards Run's goroutine and an unrecovered panic on a spawned
// goroutine would crash the whole daemon. A captured panic surfaces in phase 3
// as the run's terminal error — exactly what the sequential path produces.
func executeToolJobs(ctx context.Context, cfg LoopConfig, jobs []*toolJob) {
	var toExec []*toolJob
	for _, job := range jobs {
		if job.tool != nil {
			toExec = append(toExec, job)
		}
	}
	if len(toExec) == 0 {
		return
	}
	maxPar := cfg.MaxParallelTools
	if maxPar == 0 {
		maxPar = DefaultMaxParallelTools
	}
	if len(toExec) <= 1 || maxPar <= 1 {
		for i, job := range toExec {
			invokeToolJob(ctx, cfg, job)
			if job.panicked {
				// The old sequential panic firewall stopped execution here.
				// Settle admitted later calls without performing their effects.
				for _, pending := range toExec[i+1:] {
					pending.skipped = true
					pending.result = Result{Output: "tool call not executed after an earlier tool panic", IsError: true}
				}
				return
			}
		}
		return
	}
	sem := make(chan struct{}, maxPar)
	var wg sync.WaitGroup
	for _, job := range toExec {
		wg.Add(1)
		go func(job *toolJob) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					job.panicked = true
					job.invokeErr = fmt.Errorf("%w: %v", ErrPanic, r)
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			invokeToolJob(ctx, cfg, job)
		}(job)
	}
	wg.Wait()
}

// invokeToolJob runs ONE tool with its per-call wall-clock (M34) — bounding this
// invocation without bounding the whole run. Whether the tool's OWN deadline
// fired is captured BEFORE cancelling: calling cancel first would flip a
// not-yet-expired ctx to Canceled and mask the distinction. It keys on the
// context's state rather than the returned error so a tool that wraps its error
// without the DeadlineExceeded sentinel (e.g. the warden's "context deadline
// exceeded" string) is still classified cleanly.
func invokeToolJob(ctx context.Context, cfg LoopConfig, job *toolJob) {
	toolCtx := toolapi.WithCorrelation(ctx, cfg.CorrelationID)
	execution := toolpipeline.Execute(toolCtx, job.tool, job.tc.Input, cfg.ToolTimeout, func(panicValue any) error {
		return fmt.Errorf("%w: %v", ErrPanic, panicValue)
	})
	job.result, job.invokeErr = execution.Result, execution.Err
	if execution.PanicValue != nil {
		job.panicked = true
	}
	job.toolTimedOut = execution.TimedOut
}

// finalizeToolJobs is phase 3: classify each outcome, journal tool.result, and
// append the tool turns to `messages` in the ORIGINAL call order — so the
// conversation matches a fully sequential build regardless of how phase 2
// interleaved. Returns the extended conversation, and an error only for
// run-terminal conditions (a tool panic, the run context ending, a failed
// journal publish); a tool that merely failed becomes an error Result the model
// can react to, and the run continues.
func (s *runState) finalizeToolJobs(ctx context.Context, jobs []*toolJob, iter int, messages []Message) ([]Message, error) {
	// Executed parallel siblings have already completed. A run-terminal failure
	// must settle the whole admitted batch before task.failed, and must not
	// trigger runtime bookkeeping/automation on the way out.
	var terminalErr, auditErr error
	for _, job := range jobs {
		if job.panicked {
			terminalErr = errors.Join(terminalErr, job.invokeErr)
		}
	}
	observeCancellation := func() {
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(terminalErr, ctxErr) {
			terminalErr = errors.Join(terminalErr, ctxErr)
		}
	}
	observeCancellation()
	for _, job := range jobs {
		if job.memoSource != nil {
			job.result = job.memoSource.result
		}
		if job.tool != nil {
			switch {
			case job.panicked:
				job.result = Result{Output: job.invokeErr.Error(), IsError: true}
			case job.invokeErr == nil:
				// job.result already holds the tool's output.
			case ctx.Err() != nil:
				// The RUN context itself ended (operator halt, M32 cancel, or the
				// M31 per-run deadline) — a run-level terminal, not a tool fault.
				// Propagate so the run fails with the correct reason instead of
				// limping on.
				job.result = Result{Output: ctx.Err().Error(), IsError: true}
			case job.toolTimedOut:
				// The tool overran its own budget while the run is fine: hand the
				// model a clear error and keep the run going.
				job.result = Result{
					Output:  fmt.Sprintf("tool %q exceeded its %s timeout", job.tc.Name, s.cfg.ToolTimeout),
					IsError: true,
				}
			default:
				job.result = Result{Output: job.invokeErr.Error(), IsError: true}
			}
			if job.memoEligible && !job.result.IsError {
				s.cfg.ToolMemo.Set(job.tc.Name, job.tc.Input, job.result)
			}
		}

		modelOutput := job.result.Output
		observationDelta := false
		if s.cfg.ObservationDeltas && !job.result.IsError {
			key := job.tc.Name + "\x00" + string(job.tc.Input)
			if prev, ok := s.observations[key]; ok {
				if delta, changed := DiffObservation(prev, job.result.Output); changed {
					modelOutput = delta
					observationDelta = true
				}
			}
			s.observations[key] = job.result.Output
		}

		observationBoundary := ObservationBoundaryForTool(job.tc.Name, job.result, modelOutput)
		if observationBoundary.Trust == ObservationUntrusted {
			s.untrustedTaint = MergeUntrustedObservationTaint(s.untrustedTaint, observationBoundary)
			if observationBoundary.DirectiveLike {
				// Remember WHEN the directive-like observation arrived so the gate
				// can decay after directiveWindow iterations.
				s.directiveObsIter = iter
			}
			modelOutput = RenderObservationForModel(job.tc.Name, observationBoundary, modelOutput)
		}

		// Offload a large output out of the journal event (SPEC-04 §3.6 /
		// SPEC-01 §10.2): the event carries a preview + raw_ref. The model gets
		// the raw output by default, or the observation delta when the optional
		// CH-04 delta layer is enabled for a repeated observation.
		eventOutput, rawRef, fullBytes, offloaded := offloadToolOutput(s.cfg.Artifacts, s.cfg.ArtifactThreshold, job.result.Output)
		resultPayload := map[string]any{
			"tool":               job.tc.Name,
			"call_id":            job.tc.ID,
			"output":             eventOutput,
			"error":              job.result.IsError,
			"observation_trust":  observationBoundary.Trust,
			"observation_source": observationBoundary.Source,
			"directive_like":     observationBoundary.DirectiveLike,
			"directive_matches":  observationBoundary.Matches,
		}
		if offloaded {
			resultPayload["raw_ref"] = rawRef
			resultPayload["output_bytes"] = fullBytes
		}
		if observationDelta {
			resultPayload["observation_delta"] = true
			resultPayload["model_output_bytes"] = len(modelOutput)
			resultPayload["raw_output_bytes"] = len(job.result.Output)
		}
		if job.memoHit {
			resultPayload["memo_hit"] = true
		}
		if job.skipped {
			resultPayload["not_executed"] = true
		}
		if _, err := s.publish(event.KindToolResult, "tool", resultPayload); err != nil {
			auditErr = errors.Join(auditErr, fmt.Errorf("agent: publish tool.result: %w", err))
			continue // retain causes and attempt the remaining terminal records
		}
		observeCancellation()
		if s.cfg.ToolResultHook != nil && job.tool != nil && !job.skipped && terminalErr == nil && auditErr == nil {
			s.cfg.ToolResultHook(ctx, job.tc, job.result)
		}

		messages = append(messages, Message{
			Role:       RoleTool,
			Content:    modelOutput,
			ToolCallID: job.tc.ID,
		})
	}
	observeCancellation()
	if terminalErr != nil || auditErr != nil {
		return nil, errors.Join(terminalErr, auditErr)
	}
	return messages, nil
}
