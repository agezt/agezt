// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/retry"
)

// execute runs a prepared delegation's child loop to completion and releases
// the child's own bookkeeping.
func (s *Service) execute(p *Plan) (string, error) {
	// This child may itself delegate; release its own fan-out tally and steering
	// control when it returns so the tallies don't accumulate across a long-lived
	// daemon.
	defer func() {
		s.mu.Lock()
		delete(s.fanout, p.ChildCorr)
		s.mu.Unlock()
		s.host.UnregisterSteer(p.ChildCorr)
	}()

	var activatedSkillIDs []string
	runOnce := func() (string, error) {
		// Shared governance/capacity base (LD-1): identical to the daemon's
		// root run's, so a delegated run gets the same cost accounting
		// (CostFn — without it the per-run spend ceiling below is inert),
		// artifact offload, context compaction, and tool policy.
		lc := s.host.BaseLoopConfig(p.ChildCtx, p.ChildCorr, p.SubModel)
		system, skills, task := s.deps.Prompt.BuildSubagent(p.ChildCtx, p.ChildCorr, p.Actor, p.Task, p.System)
		activatedSkillIDs = AppendUniqueStrings(activatedSkillIDs, skills...)
		lc.TaskType = p.TaskType     // M705: route the sub-agent (chain supplies fallbacks)
		lc.ModelChain = p.ModelChain // M787: the named agent's own fallbacks win
		lc.Agent = p.AgentSlug
		lc.AgentDailyCeilingMc = p.AgentDailyMc
		lc.WakeSource = "subagent"
		lc.WakeReason = "delegation"
		lc.ParentCorrelation = p.ParentCorr
		lc.System = system
		lc.MaxRunCostMicrocents = p.MaxRunCost
		lc.Actor = p.Actor
		lc.CorrelationID = p.ChildCorr
		lc.Steer = p.Steer // M631: individual sub-agent steering
		return s.deps.RunChild(p.ChildCtx, lc, task)
	}
	answer, err := runOnce()
	if err != nil && p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		max := p.RetryPolicy.MaxAttempts
		if max > 10 {
			max = 10
		}
		for attempt := 1; err != nil && attempt < max && retry.Retryable(s.reason(err), p.RetryPolicy.RetryOn); attempt++ {
			delay := retry.Delay(*p.RetryPolicy, attempt)
			_, _ = s.host.Publish(event.Spec{
				Subject:       "agent." + p.AgentSlug + ".retry",
				Kind:          event.KindAgentRetry,
				Actor:         "agent-retry",
				CorrelationID: p.ChildCorr,
				Payload: map[string]any{
					"agent":        p.AgentSlug,
					"attempt":      attempt,
					"next_attempt": attempt + 1,
					"max_attempts": max,
					"reason":       s.reason(err),
					"error":        err.Error(),
					"delay_ms":     int64(delay / time.Millisecond),
					"subagent":     true,
				},
			})
			if delay > 0 {
				if werr := s.deps.sleep(p.ChildCtx, delay); werr != nil {
					return "", werr
				}
			}
			answer, err = runOnce()
		}
	}
	if len(activatedSkillIDs) > 0 {
		s.host.RecordSkillOutcome(p.ChildCorr, activatedSkillIDs, err == nil)
	}
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", p.ChildCorr, err)
	}
	// The daemon advances the durable lifecycle for the finished child; the
	// engine owns the delegation, the daemon owns the run lifecycle.
	s.host.CompleteChildLifecycle(p.ChildCtx, p.ChildCorr)
	return answer, nil
}

// reason classifies a child failure with the daemon's own halt sentinel.
func (s *Service) reason(err error) string {
	return retry.Reason(err, s.host.HaltedError())
}

// spawn starts a prepared delegation on its own goroutine and returns the
// child's correlation id (its spawn id) immediately.
//
// The child's lifetime is detached from the spawning TOOL CALL (whose context
// ends when delegate returns) but stays bounded by the daemon: its cancel is
// registered in the live-run registry (so Halt and CancelRun reach it) and the
// parent run's cleanup cancels any un-awaited children, so a spawn never
// outlives its delegation tree.
func (s *Service) spawn(p *Plan) (string, error) {
	reg := s.deps.Registry
	if reg == nil {
		return "", fmt.Errorf("async delegation is not wired: no live-run registry")
	}
	// WithoutCancel keeps the run-stamped values (depth, root, actor, child
	// correlation, memory scope, workdir) while dropping the tool-call
	// deadline/cancel; the fresh WithCancel re-attaches a kill switch owned
	// by the daemon instead of the spawning call.
	childCtx, cancel := context.WithCancel(context.WithoutCancel(p.ChildCtx))
	p.ChildCtx = childCtx
	h := &spawnHandle{
		parentCorr: p.ParentCorr,
		rootCorr:   p.RootCorr,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	reg.Lock()
	if reg.Halted() {
		// Halt won the race after prepare's check: don't start a goroutine
		// Halt's sweep of the registry can no longer see.
		reg.Unlock()
		cancel()
		return "", s.host.HaltedError()
	}
	s.mu.Lock()
	s.spawns[p.ChildCorr] = h
	s.mu.Unlock()
	reg.Register(p.ChildCorr, cancel)
	reg.Add(1) // Close drains async spawns like any in-flight run (M883)
	reg.Unlock()
	go func() {
		defer reg.Done()
		defer cancel()
		answer, err := s.execute(p)
		h.answer, h.err = answer, err
		reg.Lock()
		reg.Unregister(p.ChildCorr)
		reg.Unlock()
		// Announce completion under the parent correlation BEFORE releasing
		// awaiters, so by the time delegate_await returns the outcome is
		// already durably journaled (subscribable by the UI as a push signal).
		payload := map[string]any{
			"child_correlation": p.ChildCorr,
			"ok":                err == nil,
			"async":             true,
		}
		if err != nil {
			payload["error"] = err.Error()
		} else {
			payload["chars"] = len(answer)
		}
		_, _ = s.host.Publish(event.Spec{
			Subject:       "agent." + p.Actor + ".subagent",
			Kind:          event.KindSubAgentCompleted,
			Actor:         p.Actor,
			CorrelationID: p.LinkCorr,
			Payload:       payload,
		})
		close(h.done)
	}()
	return p.ChildCorr, nil
}
