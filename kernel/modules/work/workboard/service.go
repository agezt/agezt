// SPDX-License-Identifier: MIT

package workboard

// The board's SERVICE: the lifecycle verbs, over the contract's host port.
//
// The verbs used to live in the daemon (kernel/runtime/workboard.go): the
// daemon held the store, called the transition and published the event. The
// DATA was already here — the store, the transitions and the retry rule are
// the module's implementation — so the daemon was the only place that knew
// what a task's lifecycle MEANT.
//
// The service closes that split: the module that keeps the data also keeps
// the operations over it, and the daemon holds the module's face
// (kernel/modules/work/api.Service) and forwards to it.
//
// The store port stays the CONTRACT's host port (workboardapi.HostStore): the
// file it lives in is the host's business, and the host may not name the
// module that keeps it. The service is built over that port by the
// composition root, which is the one place that knows both.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

// service is the board's own face over the host's store port.
type service struct {
	store workboardapi.HostStore
	deps  api.Dependencies
}

// NewService builds the board's service over the host's store port.
//
// A nil store is not an error here: the board REFUSES every verb, and the
// refusal is the contract's row (workboardapi.ErrNoStore) — the same answer
// the daemon gives when it holds no board.
func NewService(store workboardapi.HostStore, deps api.Dependencies) api.Service {
	return &service{store: store, deps: deps}
}

// storeOrRefusal returns the store, or the contract's refusal.
func (s *service) storeOrRefusal() (workboardapi.HostStore, error) {
	if s.store == nil {
		return nil, workboardapi.ErrNoStore
	}
	return s.store, nil
}

func (s *service) Create(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, false, err
	}
	t, created, err := store.Create(spec, time.Now())
	if err != nil {
		return workboardapi.Task{}, false, err
	}
	if created {
		s.publish(corr, event.KindWorkboardTaskCreated, t, "created", nil)
	}
	return t, created, nil
}

func (s *service) Claim(corr, id, agent, runID string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Claim(id, agent, runID, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskClaimed, t, "claimed", map[string]any{"agent": agent, "run_id": runID})
	return t, nil
}

func (s *service) Heartbeat(corr, id, agent, runID string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Heartbeat(id, agent, runID, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskHeartbeat, t, "heartbeat", nil)
	return t, nil
}

func (s *service) Comment(corr, id, author, body string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Comment(id, author, body, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskCommented, t, "commented", map[string]any{"author": author})
	return t, nil
}

func (s *service) Block(corr, id, actor, reason string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Block(id, actor, reason, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "blocked", map[string]any{"actor": actor, "reason": reason})
	return t, nil
}

func (s *service) Fail(corr, id, actor, reason string) (workboardapi.Task, workboardapi.RetryDecision, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, workboardapi.RetryDecision{}, err
	}
	t, decision, err := store.Fail(id, actor, reason, time.Now())
	if err != nil {
		return workboardapi.Task{}, workboardapi.RetryDecision{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, decision.Action, map[string]any{
		"actor":         actor,
		"reason":        reason,
		"failure_count": decision.FailureCount,
		"max_attempts":  decision.MaxAttempts,
		"next_attempt":  decision.NextAttempt,
		"retry":         decision.Retry,
		"exhausted":     decision.Exhausted,
		"escalate_to":   decision.EscalateTo,
	})
	return t, decision, nil
}

func (s *service) Unblock(corr, id, actor string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Unblock(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "unblocked", map[string]any{"actor": actor})
	return t, nil
}

func (s *service) SetRetryPolicy(corr, id, actor string, policy *workboardapi.RetryPolicy) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.SetRetryPolicy(id, actor, policy, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	extra := map[string]any{"actor": actor}
	if policy != nil {
		extra["max_attempts"] = policy.MaxAttempts
		extra["escalate_to"] = policy.EscalateTo
	} else {
		extra["cleared"] = true
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "retry_policy", extra)
	return t, nil
}

func (s *service) Complete(corr, id, actor string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Complete(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "completed", map[string]any{"actor": actor})
	// An ungated task completing normally still rolls up into any OKR that links it.
	if s.deps.TaskCompleted != nil {
		s.deps.TaskCompleted(corr, id)
	}
	return t, nil
}

func (s *service) Review(corr, id, actor, summary string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Review(id, actor, summary, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "review", map[string]any{"actor": actor})
	return t, nil
}

func (s *service) Archive(corr, id, actor string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Archive(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, "archived", map[string]any{"actor": actor})
	return t, nil
}

func (s *service) Link(corr, id, typ, target string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Link(id, typ, target, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskLinked, t, "linked", map[string]any{"type": typ, "target": target})
	return t, nil
}

func (s *service) AddDependency(corr, id, dependsOn string) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.AddDependency(id, dependsOn, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	s.publish(corr, event.KindWorkboardTaskDependency, t, "dependency_added", map[string]any{"depends_on": dependsOn})
	return t, nil
}

func (s *service) ReclaimStale(corr, id, actor string, staleAfter time.Duration) (workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.ReclaimStale(id, actor, staleAfter, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	action := "reclaimed"
	decision := workboardapi.RetryDecisionFor(t, "stale heartbeat")
	if decision.Exhausted {
		action = "escalate"
	}
	s.publish(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
		"actor":          actor,
		"stale_after_ms": staleAfter.Milliseconds(),
		"failure_count":  decision.FailureCount,
		"max_attempts":   decision.MaxAttempts,
		"next_attempt":   decision.NextAttempt,
		"retry":          decision.Retry,
		"exhausted":      decision.Exhausted,
		"escalate_to":    decision.EscalateTo,
	})
	return t, nil
}

func (s *service) SweepStaleClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboardapi.Task, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return nil, err
	}
	tasks, err := store.SweepStaleClaims(actor, staleAfter, limit, time.Now())
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		action := "reclaimed"
		decision := workboardapi.RetryDecisionFor(t, "stale heartbeat")
		if decision.Exhausted {
			action = "escalate"
		}
		s.publish(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
			"actor":          actor,
			"stale_after_ms": staleAfter.Milliseconds(),
			"sweep":          true,
			"failure_count":  decision.FailureCount,
			"max_attempts":   decision.MaxAttempts,
			"next_attempt":   decision.NextAttempt,
			"retry":          decision.Retry,
			"exhausted":      decision.Exhausted,
			"escalate_to":    decision.EscalateTo,
		})
	}
	return tasks, nil
}

// publish says what happened to a task on the host's bus.
//
// The subject, the kind and the payload are the BOARD's own: the host's port
// carries the event, but the board decides what the event says. The payload's
// shape is the contract's row (workboardapi.TaskEventPayload), because the
// host's proof path publishes the same shape and the tools read it.
//
// A host with no bus (no publish port) is quiet rather than broken — the task
// still moved, and the caller still holds the answer.
func (s *service) publish(corr string, kind event.Kind, t workboardapi.Task, action string, extra map[string]any) {
	if s.deps.Publish == nil {
		return
	}
	s.deps.Publish(event.Spec{
		Subject:       "workboard." + t.ID,
		Kind:          kind,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       workboardapi.TaskEventPayload(t, action, extra),
	})
}
