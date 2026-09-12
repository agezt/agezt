// SPDX-License-Identifier: MIT

// Agent run-tools: gateToolCalls (per-call policy + capability matching).
// Code extracted from run_tools.go during the Day-127 god-file split.
// Public API unchanged.
package agent


import (
	"context"
	"fmt"
	"sync"

	"github.com/agezt/agezt/kernel/event"
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

		tool, ok := s.cfg.Tools[tc.Name]
		if !ok {
			job.result = Result{
				Output:  fmt.Sprintf("tool %q is not available", tc.Name),
				IsError: true,
			}
			continue
		}
		def := tool.Definition()
		if err := ValidateToolInput(def, tc.Input); err != nil {
			job.result = Result{
				Output:  "tool call rejected by schema: " + err.Error(),
				IsError: true,
			}
			continue
		}

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
		verdict := PolicyVerdict{Allow: true, Capability: tc.Name, Reason: "no policy configured"}
		if s.cfg.Policy != nil {
			policyCtx := WithPolicyToolDef(ctx, def)
			// Thread the taint with DirectiveLike scoped to the causal window.
			scopedTaint := s.untrustedTaint
			scopedTaint.DirectiveLike = s.directiveActive(iter)
			policyCtx = WithUntrustedObservationTaint(policyCtx, scopedTaint)
			verdict = s.cfg.Policy(policyCtx, tc)
		}
		if _, err := s.publish(event.KindPolicyDecision, "policy", policyDecisionPayload(tc, verdict)); err != nil {
			return nil, fmt.Errorf("agent: publish policy.decision: %w", err)
		}

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
		if _, err := s.publish(event.KindToolInvoked, "tool", map[string]any{
			"tool":    tc.Name,
			"call_id": tc.ID,
			"input":   tc.Input,
		}); err != nil {
			return nil, fmt.Errorf("agent: publish tool.invoked: %w", err)
		}
		job.tool = tool
	}
	return jobs, nil
}

// policyDecisionPayload renders one gate decision for the journal. Split out so
// the 24-field map doesn't dominate the gating logic it belongs to.
func policyDecisionPayload(tc ToolCall, v PolicyVerdict) map[string]any {
	return map[string]any{
		"tool":                  tc.Name,
		"call_id":               tc.ID,
		"capability":            v.Capability,
		"allow":                 v.Allow,
		"reason":                v.Reason,
		"would_ask":             v.WouldAsk,
		"hard_denied":           v.HardDenied,
		"effect_class":          v.EffectClass,
		"affected_resources":    v.AffectedResources,
		"epistemic_action":      v.EpistemicAction,
		"epistemic_reason":      v.EpistemicReason,
		"epistemic_signals":     v.EpistemicSignals,
		"epistemic_confidence":  v.EpistemicConfidence,
		"failure_matches":       v.FailureMatches,
		"weighted_failures":     v.WeightedFailures,
		"schema_hash":           v.SchemaHash,
		"input_shape":           v.InputShape,
		"temporal_sensitive":    v.TemporalSensitive,
		"novel_tool":            v.NovelTool,
		"untrusted_observation": v.UntrustedObservation,
		"observation_sources":   v.ObservationSources,
		"directive_like":        v.ObservationDirectiveLike,
		"directive_matches":     v.ObservationDirectiveMatches,
	}
}

// executeToolJobs is phase 2: run every gated-allowed job. A single call (the
// common case) or disabled parallelism runs inline on the caller's goroutine,
// where Run's panic firewall (M168) covers it; a multi-call turn fans out up to
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
		for _, job := range toExec {
			invokeToolJob(ctx, cfg, job)
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
	toolCtx := WithCorrelation(ctx, cfg.CorrelationID)
	var toolCancel context.CancelFunc
	if cfg.ToolTimeout > 0 {
		toolCtx, toolCancel = context.WithTimeout(toolCtx, cfg.ToolTimeout)
	}
	job.result, job.invokeErr = job.tool.Invoke(toolCtx, job.tc.Input)
	job.toolTimedOut = cfg.ToolTimeout > 0 && toolCtx.Err() == context.DeadlineExceeded
	if toolCancel != nil {
		toolCancel()
	}
}

// finalizeToolJobs is phase 3: classify each outcome, journal tool.result, and
// append the tool turns to `messages` in the ORIGINAL call order — so the
// conversation matches a fully sequential build regardless of how phase 2
// interleaved. Returns the extended conversation, and an error only for
// run-terminal conditions (a tool panic, the run context ending, a failed
// journal publish); a tool that merely failed becomes an error Result the model
// can react to, and the run continues.
func (s *runState) finalizeToolJobs(ctx context.Context, jobs []*toolJob, iter int, messages []Message) ([]Message, error) {
	for _, job := range jobs {
		if job.memoSource != nil {
			job.result = job.memoSource.result
		}
		if job.tool != nil {
			switch {
			case job.panicked:
				return nil, job.invokeErr
			case job.invokeErr == nil:
				// job.result already holds the tool's output.
			case ctx.Err() != nil:
				// The RUN context itself ended (operator halt, M32 cancel, or the
				// M31 per-run deadline) — a run-level terminal, not a tool fault.
				// Propagate so the run fails with the correct reason instead of
				// limping on.
				return nil, ctx.Err()
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
		if _, err := s.publish(event.KindToolResult, "tool", resultPayload); err != nil {
			return nil, fmt.Errorf("agent: publish tool.result: %w", err)
		}
		if s.cfg.ToolResultHook != nil && job.tool != nil {
			s.cfg.ToolResultHook(ctx, job.tc, job.result)
		}

		messages = append(messages, Message{
			Role:       RoleTool,
			Content:    modelOutput,
			ToolCallID: job.tc.ID,
		})
	}
	return messages, nil
}
