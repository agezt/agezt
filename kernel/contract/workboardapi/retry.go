// SPDX-License-Identifier: MIT

package workboardapi

import "strings"

// RetryDecisionFor is the retry rule: given a task and the reason it failed,
// what happens next — retry (and when), escalate, or block.
//
// The rule is PURE and it reads the task, so it travels with the task: it
// lives in the contract layer and the module delegates to it, so there is one
// rule. A second copy would drift, and the drift would show up as a task that
// retries forever next to a task that gives up after one failure.
func RetryDecisionFor(t Task, reason string) RetryDecision {
	return retryDecision(t.RetryPolicy, FailedAttemptCount(t), strings.TrimSpace(reason))
}

// retryDecision is the rule's body: a policy with no budget blocks, a budget
// that is not spent yet retries, and a spent budget escalates.
func retryDecision(policy *RetryPolicy, failureCount int, reason string) RetryDecision {
	decision := RetryDecision{FailureCount: failureCount, Reason: reason, Action: "block"}
	policy = NormalizeRetryPolicy(policy)
	if policy == nil || policy.MaxAttempts <= 0 {
		return decision
	}
	cp := *policy
	decision.Policy = &cp
	decision.MaxAttempts = cp.MaxAttempts
	decision.EscalateTo = cp.EscalateTo
	if failureCount < cp.MaxAttempts {
		decision.Retry = true
		decision.NextAttempt = failureCount + 1
		decision.Action = "retry"
		return decision
	}
	decision.Exhausted = true
	decision.Action = "escalate"
	return decision
}

// NormalizeRetryPolicy drops a policy that carries no budget: a policy with
// no attempts and no escalation target is not a policy. The escalation target
// is trimmed — it names an agent, and a padded name is a different agent.
//
// The rule lives here because the retry rule reads it and the store that
// writes a task's policy normalizes with it: two copies would let a task be
// stored with a policy the rule does not recognise.
func NormalizeRetryPolicy(policy *RetryPolicy) *RetryPolicy {
	if policy == nil {
		return nil
	}
	cp := *policy
	cp.EscalateTo = strings.TrimSpace(cp.EscalateTo)
	if cp.MaxAttempts <= 0 && cp.EscalateTo == "" {
		return nil
	}
	return &cp
}
