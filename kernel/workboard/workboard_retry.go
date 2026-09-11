// SPDX-License-Identifier: MIT

package workboard

// Retry-policy helpers: FailedAttemptCount + RetryDecisionFor +
// attemptCountsAsFailure + markFailedAttempt + applyRetryPolicyDecision +
// retryDecision + reclaimStaleTask + cloneTask. Carved out of
// workboard.go during the Day 36 god file split #1.

import (
	"fmt"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/ulid"
)


func FailedAttemptCount(t Task) int {
	n := 0
	for _, a := range t.Attempts {
		if attemptCountsAsFailure(a.Status) {
			n++
		}
	}
	return n
}

func RetryDecisionFor(t Task, reason string) RetryDecision {
	return retryDecision(t.RetryPolicy, FailedAttemptCount(t), strings.TrimSpace(reason))
}

func attemptCountsAsFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stale":
		return true
	default:
		return false
	}
}

func markFailedAttempt(t *Task, reason string, ts int64) {
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" {
			t.Attempts[i].Status = "failed"
			t.Attempts[i].FinishedMS = ts
			t.Attempts[i].Summary = reason
			return
		}
	}
	agent, runID := "", ""
	if t.Claim != nil {
		agent = t.Claim.Agent
		runID = t.Claim.RunID
	}
	t.Attempts = append(t.Attempts, Attempt{
		ID:         ulid.New(),
		Agent:      agent,
		RunID:      runID,
		Status:     "failed",
		StartedMS:  ts,
		FinishedMS: ts,
		Summary:    reason,
	})
}

func applyRetryPolicyDecision(t *Task, actor, reason string, ts int64) RetryDecision {
	decision := RetryDecisionFor(*t, reason)
	switch {
	case decision.Retry:
		t.Status = StatusReady
		t.BlockReason = ""
		t.Comments = append(t.Comments, Comment{
			ID:        ulid.New(),
			Author:    actor,
			Body:      fmt.Sprintf("retry scheduled: attempt %d/%d after failure: %s", decision.NextAttempt, decision.MaxAttempts, reason),
			CreatedMS: ts,
		})
	case decision.Exhausted:
		t.Status = StatusBlocked
		t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts: %s", decision.FailureCount, decision.MaxAttempts, reason)
		if decision.EscalateTo != "" {
			t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts; escalate to %s: %s", decision.FailureCount, decision.MaxAttempts, decision.EscalateTo, reason)
		}
		body := "escalated: " + t.BlockReason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
	default:
		t.Status = StatusBlocked
		t.BlockReason = reason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "failed: " + reason, CreatedMS: ts})
	}
	return decision
}

func retryDecision(policy *RetryPolicy, failureCount int, reason string) RetryDecision {
	decision := RetryDecision{FailureCount: failureCount, Reason: reason, Action: "block"}
	policy = normalizeRetryPolicy(policy)
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

func reclaimStaleTask(t *Task, actor string, staleAfter time.Duration, ts int64) error {
	if t.Claim == nil {
		return ErrNotClaimed
	}
	if ts-t.Claim.HeartbeatMS < staleAfter.Milliseconds() {
		return ErrClaimFresh
	}
	old := *t.Claim
	t.Status = StatusReady
	t.Claim = nil
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" && strings.EqualFold(t.Attempts[i].Agent, old.Agent) && (old.RunID == "" || t.Attempts[i].RunID == old.RunID) {
			t.Attempts[i].Status = "stale"
			t.Attempts[i].FinishedMS = ts
			t.Attempts[i].Summary = "reclaimed after stale heartbeat"
			break
		}
	}
	body := "reclaimed stale claim"
	if old.Agent != "" {
		body += " from " + old.Agent
	}
	t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
	if decision := RetryDecisionFor(*t, "stale heartbeat"); decision.Exhausted {
		t.Status = StatusBlocked
		t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts: stale heartbeat", decision.FailureCount, decision.MaxAttempts)
		if decision.EscalateTo != "" {
			t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts; escalate to %s: stale heartbeat", decision.FailureCount, decision.MaxAttempts, decision.EscalateTo)
		}
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "escalated: " + t.BlockReason, CreatedMS: ts})
	} else if decision.Retry {
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: fmt.Sprintf("retry scheduled: attempt %d/%d after stale heartbeat", decision.NextAttempt, decision.MaxAttempts), CreatedMS: ts})
	}
	return nil
}

func cloneTask(t Task) Task {
	t.Tags = append([]string(nil), t.Tags...)
	t.Artifacts = append([]string(nil), t.Artifacts...)
	t.Criteria = append([]proof.Criterion(nil), t.Criteria...)
	if t.Proof != nil {
		cp := t.Proof.Clone()
		t.Proof = &cp
	}
	if t.RetryPolicy != nil {
		cp := *t.RetryPolicy
		t.RetryPolicy = &cp
	}
	t.Dependencies = append([]Dependency(nil), t.Dependencies...)
	t.Attempts = append([]Attempt(nil), t.Attempts...)
	t.Comments = append([]Comment(nil), t.Comments...)
	t.Links = append([]Link(nil), t.Links...)
	if t.Claim != nil {
		cp := *t.Claim
		t.Claim = &cp
	}
	return t
}

