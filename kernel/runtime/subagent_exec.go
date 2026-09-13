// SPDX-License-Identifier: MIT

package runtime

// executeSubAgent runs a prepared delegation's child loop to completion.
// Carved out of subagent_prep.go during the Day 152 god-file split so the
// prep file can focus on building the subAgentPrep + validating the
// delegation manager.
// Public API unchanged.

import (
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/delegation"
	"github.com/agezt/agezt/kernel/event"
)
func (k *Kernel) executeSubAgent(p *subAgentPrep) (string, error) {
	// This child may itself delegate; release its own fan-out tally and steering
	// control when it returns so the maps don't accumulate across a long-lived
	// kernel.
	defer func() {
		k.fanoutMu.Lock()
		delete(k.fanout, p.childCorr)
		k.fanoutMu.Unlock()
		k.steersMu.Lock()
		delete(k.steers, p.childCorr)
		k.steersMu.Unlock()
	}()

	var activatedSkillIDs []string
	runOnce := func() (string, []string, error) {
		// Shared governance/capacity base (LD-1): identical to RunWith's, so a
		// delegated run gets the same cost accounting (CostFn — without it the
		// per-run spend ceiling below is inert), artifact offload, context
		// compaction, and tool policy as a root run.
		lc := k.buildLoopConfig(p.childCtx, p.childCorr, p.subModel)
		system, skills, task := k.subAgentInjectedSystem(p.childCtx, p.childCorr, p.actor, p.task, p.system)
		activatedSkillIDs = delegation.AppendUniqueStrings(activatedSkillIDs, skills...)
		lc.TaskType = p.taskType     // M705: route the sub-agent (chain supplies fallbacks)
		lc.ModelChain = p.modelChain // M787: the named agent's own fallbacks win
		lc.Agent = p.agentSlug
		lc.AgentDailyCeilingMc = p.agentDailyMc
		lc.WakeSource = "subagent"
		lc.WakeReason = "delegation"
		lc.ParentCorrelation = p.parentCorr
		lc.System = system
		lc.MaxRunCostMicrocents = p.maxRunCost
		lc.Actor = p.actor
		lc.CorrelationID = p.childCorr
		lc.Steer = p.rc // M631: individual sub-agent steering
		answer, err := agent.Run(p.childCtx, lc, task)
		return answer, skills, err
	}
	answer, _, err := runOnce()
	if err != nil && p.retryPolicy != nil && p.retryPolicy.MaxAttempts > 1 {
		max := p.retryPolicy.MaxAttempts
		if max > 10 {
			max = 10
		}
		for attempt := 1; err != nil && attempt < max && agentRetryable(retryReason(err), p.retryPolicy.RetryOn); attempt++ {
			delay := retryDelay(*p.retryPolicy, attempt)
			_, _ = k.bus.Publish(event.Spec{
				Subject:       "agent." + p.agentSlug + ".retry",
				Kind:          event.KindAgentRetry,
				Actor:         "agent-retry",
				CorrelationID: p.childCorr,
				Payload: map[string]any{
					"agent":        p.agentSlug,
					"attempt":      attempt,
					"next_attempt": attempt + 1,
					"max_attempts": max,
					"reason":       retryReason(err),
					"error":        err.Error(),
					"delay_ms":     int64(delay / time.Millisecond),
					"subagent":     true,
				},
			})
			if delay > 0 {
				t := time.NewTimer(delay)
				select {
				case <-p.childCtx.Done():
					if !t.Stop() {
						select {
						case <-t.C:
						default:
						}
					}
					return "", p.childCtx.Err()
				case <-t.C:
				}
			}
			answer, _, err = runOnce()
		}
	}
	if k.forge != nil && len(activatedSkillIDs) > 0 {
		k.forge.RecordOutcome(p.childCorr, activatedSkillIDs, err == nil)
	}
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", p.childCorr, err)
	}
	// Day 33: lifecycle-advance lives in the runexec sub-package;
	// call through the public wrapper.
	k.CompleteAgentLifecycle(p.childCtx, p.childCorr)
	return answer, nil
}
