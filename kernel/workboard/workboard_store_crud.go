// SPDX-License-Identifier: MIT

package workboard

// Store CRUD: Get + List + Claim + Heartbeat + Comment + Block + Fail +
// Unblock + SetRetryPolicy + Complete + Prove + the reconcileCriteria
// doc comment block. (reconcileCriteria itself lives in
// workboard_helpers.go.) Carved out of workboard.go during the Day 36
// god file split #1.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/ulid"
)

func (s *Store) Get(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(id); t != nil {
		return cloneTask(*t), true
	}
	return Task{}, false
}

func (s *Store) List(f Filter) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if !f.IncludeArchived && t.Status == StatusArchived {
			continue
		}
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if f.Tenant != "" && t.Tenant != f.Tenant {
			continue
		}
		if f.Assignee != "" && !strings.EqualFold(t.Assignee, f.Assignee) {
			continue
		}
		out = append(out, cloneTask(*t))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ia, ja := out[i].Status == StatusArchived, out[j].Status == StatusArchived
		if ia != ja {
			return !ia
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].UpdatedMS != out[j].UpdatedMS {
			return out[i].UpdatedMS > out[j].UpdatedMS
		}
		if statusOrder[out[i].Status] != statusOrder[out[j].Status] {
			return statusOrder[out[i].Status] < statusOrder[out[j].Status]
		}
		return out[i].ID < out[j].ID
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

func (s *Store) Claim(id, agent, runID string, now time.Time) (Task, error) {
	agent = strings.TrimSpace(agent)
	runID = strings.TrimSpace(runID)
	if id == "" || agent == "" {
		return Task{}, errors.New("workboard: claim requires id and agent")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Status == StatusArchived || t.Status == StatusDone {
			return fmt.Errorf("workboard: cannot claim %s task", t.Status)
		}
		if t.Claim != nil && !strings.EqualFold(t.Claim.Agent, agent) {
			return ErrClaimConflict
		}
		t.Status = StatusRunning
		if t.Assignee == "" {
			t.Assignee = agent
		}
		t.Claim = &Claim{Agent: agent, RunID: runID, ClaimedMS: ts, HeartbeatMS: ts}
		t.Attempts = append(t.Attempts, Attempt{ID: ulid.New(), Agent: agent, RunID: runID, Status: "running", StartedMS: ts})
		return nil
	}, now)
}

func (s *Store) Heartbeat(id, agent, runID string, now time.Time) (Task, error) {
	agent = strings.TrimSpace(agent)
	runID = strings.TrimSpace(runID)
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Claim == nil {
			return ErrNotClaimed
		}
		if agent != "" && !strings.EqualFold(t.Claim.Agent, agent) {
			return ErrClaimConflict
		}
		if runID != "" && t.Claim.RunID != "" && t.Claim.RunID != runID {
			return ErrClaimConflict
		}
		t.Claim.HeartbeatMS = ts
		return nil
	}, now)
}

func (s *Store) Comment(id, author, body string, now time.Time) (Task, error) {
	author = strings.TrimSpace(author)
	body = strings.TrimSpace(body)
	if body == "" {
		return Task{}, errors.New("workboard: comment body required")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: author, Body: body, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) Block(id, actor, reason string, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, errors.New("workboard: block reason required")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusBlocked
		t.BlockReason = reason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "blocked: " + reason, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) Fail(id, actor, reason string, now time.Time) (Task, RetryDecision, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, RetryDecision{}, errors.New("workboard: fail reason required")
	}
	var decision RetryDecision
	task, err := s.mutate(id, func(t *Task, ts int64) error {
		markFailedAttempt(t, reason, ts)
		t.Claim = nil
		decision = applyRetryPolicyDecision(t, actor, reason, ts)
		return nil
	}, now)
	if err != nil {
		return Task{}, RetryDecision{}, err
	}
	return task, decision, nil
}

func (s *Store) Unblock(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusReady
		t.BlockReason = ""
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "unblocked", CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) SetRetryPolicy(id, actor string, policy *RetryPolicy, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	policy = normalizeRetryPolicy(policy)
	if policy != nil && policy.MaxAttempts < 1 {
		return Task{}, fmt.Errorf("%w: max_attempts must be positive when policy is set", ErrInvalidPolicy)
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.RetryPolicy = policy
		body := "retry policy cleared"
		if policy != nil {
			body = fmt.Sprintf("retry policy set: max_attempts=%d", policy.MaxAttempts)
			if policy.EscalateTo != "" {
				body += " escalate_to=" + policy.EscalateTo
			}
		}
		if actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) Complete(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		// The proof gate: a task that declared acceptance criteria may only reach
		// done once a recorded Proof satisfies them. Tasks without criteria are
		// ungated and complete as before (default-allow posture).
		if len(t.Criteria) > 0 && (t.Proof == nil || !t.Proof.Satisfied()) {
			return ErrUnproven
		}
		t.Status = StatusDone
		t.CompletedMS = ts
		t.Claim = nil
		for i := len(t.Attempts) - 1; i >= 0; i-- {
			if t.Attempts[i].Status == "running" {
				t.Attempts[i].Status = "done"
				t.Attempts[i].FinishedMS = ts
				break
			}
		}
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "completed", CreatedMS: ts})
		}
		return nil
	}, now)
}

// Prove records a completion proof against the task's acceptance criteria and
// gates the transition to done on it. When the proof is satisfied (verifier
// complete AND every declared criterion met) the task moves to done; otherwise
// it lands in review with the proof attached so the operator sees exactly which
// criteria remain unmet. Prove is the only path a criteria-bearing task takes
// to reach done.
func (s *Store) Prove(id, actor string, p proof.Proof, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Status == StatusArchived {
			return fmt.Errorf("workboard: cannot prove %s task", t.Status)
		}
		if p.ProvedMS == 0 {
			p.ProvedMS = ts
		}
		// Merge the judge's per-criterion outcomes back onto the task's declared
		// criteria (matched by text so create-time order and wording are kept).
		if len(t.Criteria) > 0 {
			p.Criteria = reconcileCriteria(t.Criteria, p.Criteria)
			t.Criteria = append([]proof.Criterion(nil), p.Criteria...)
		}
		cp := p.Clone()
		t.Proof = &cp
		t.Claim = nil
		if p.Satisfied() {
			t.Status = StatusDone
			t.CompletedMS = ts
			finishRunningAttempt(t, "done", ts, "")
			if actor != "" {
				t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "proven complete", CreatedMS: ts})
			}
			return nil
		}
		t.Status = StatusReview
		// An unproven task must not carry a stale completion time (a re-prove of a
		// previously-done task can land here).
		t.CompletedMS = 0
		finishRunningAttempt(t, "review", ts, p.Verdict.Gap)
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "unproven: " + provenGapSummary(p), CreatedMS: ts})
		return nil
	}, now)
}

// reconcileCriteria returns the declared criteria in their original order with
// Met/Note taken from the judged list (matched by trimmed, case-insensitive
// text). A declared criterion the judge did not address stays unmet.
