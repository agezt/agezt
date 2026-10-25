// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime workboard: getter + state-transition wrappers +
//             publishWorkboard. Code extracted from workboard.go during the Day-121
//             god-file split. Public API unchanged.

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

// errNoWorkboard is the answer a kernel gives when it holds no task board.
//
// The board used to be opened by the kernel, so the field was never nil and
// the code never had to say what happens without one. Now the store file is
// the host's (W3.47) and a host may open a kernel without a board — and the
// answer is a refusal, not a panic.
var errNoWorkboard = errors.New("runtime: the task board is not available on this daemon (no workboard store)")

// workboardStore returns the board port, or the refusal.
func (k *Kernel) workboardStore() (workboardapi.HostStore, error) {
	if k.workboard == nil {
		return nil, errNoWorkboard
	}
	return k.workboard, nil
}

// Workboard returns the durable typed multi-agent work queue.
//
// The port is the CONTRACT's host port (workboardapi.HostStore): the kernel
// holds a port, not a module's store.
func (k *Kernel) Workboard() workboardapi.HostStore { return k.workboard }

func (k *Kernel) CreateWorkboardTask(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, false, err
	}
	t, created, err := store.Create(spec, time.Now())
	if err != nil {
		return workboardapi.Task{}, false, err
	}
	if created {
		k.publishWorkboard(corr, event.KindWorkboardTaskCreated, t, "created", nil)
	}
	return t, created, nil
}

func (k *Kernel) ClaimWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Claim(id, agent, runID, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskClaimed, t, "claimed", map[string]any{"agent": agent, "run_id": runID})
	return t, nil
}

func (k *Kernel) HeartbeatWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Heartbeat(id, agent, runID, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskHeartbeat, t, "heartbeat", map[string]any{"agent": agent, "run_id": runID})
	return t, nil
}

func (k *Kernel) CommentWorkboardTask(corr, id, author, body string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Comment(id, author, body, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskCommented, t, "commented", map[string]any{"author": author})
	return t, nil
}

func (k *Kernel) BlockWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Block(id, actor, reason, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "blocked", map[string]any{"actor": actor, "reason": reason})
	return t, nil
}

func (k *Kernel) FailWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, workboardapi.RetryDecision, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, workboardapi.RetryDecision{}, err
	}
	t, decision, err := store.Fail(id, actor, reason, time.Now())
	if err != nil {
		return workboardapi.Task{}, workboardapi.RetryDecision{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, decision.Action, map[string]any{
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

func (k *Kernel) UnblockWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Unblock(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "unblocked", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) SetWorkboardRetryPolicy(corr, id, actor string, policy *workboardapi.RetryPolicy) (workboardapi.Task, error) {
	store, err := k.workboardStore()
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
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "retry_policy", extra)
	return t, nil
}

func (k *Kernel) CompleteWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Complete(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "completed", map[string]any{"actor": actor})
	// An ungated task completing normally still rolls up into any OKR that links it.
	k.recomputeOKRForTask(corr, id)
	return t, nil
}

func (k *Kernel) ReviewWorkboardTask(corr, id, actor, summary string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Review(id, actor, summary, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "review", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) ArchiveWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Archive(id, actor, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, "archived", map[string]any{"actor": actor})
	return t, nil
}

func (k *Kernel) LinkWorkboardTask(corr, id, typ, target string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.Link(id, typ, target, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskLinked, t, "linked", map[string]any{"type": typ, "target": target})
	return t, nil
}

func (k *Kernel) AddWorkboardDependency(corr, id, dependsOn string) (workboardapi.Task, error) {
	store, err := k.workboardStore()
	if err != nil {
		return workboardapi.Task{}, err
	}
	t, err := store.AddDependency(id, dependsOn, time.Now())
	if err != nil {
		return workboardapi.Task{}, err
	}
	k.publishWorkboard(corr, event.KindWorkboardTaskDependency, t, "dependency_added", map[string]any{"depends_on": dependsOn})
	return t, nil
}

func (k *Kernel) ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (workboardapi.Task, error) {
	store, err := k.workboardStore()
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
	k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
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

func (k *Kernel) SweepStaleWorkboardClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboardapi.Task, error) {
	store, err := k.workboardStore()
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
		k.publishWorkboard(corr, event.KindWorkboardTaskUpdated, t, action, map[string]any{
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

func (k *Kernel) publishWorkboard(corr string, kind event.Kind, t workboardapi.Task, action string, extra map[string]any) {
	payload := map[string]any{
		"id":       t.ID,
		"title":    t.Title,
		"status":   string(t.Status),
		"priority": t.Priority,
		"assignee": t.Assignee,
		"tenant":   t.Tenant,
		"action":   action,
	}
	for key, val := range extra {
		if val != "" {
			payload[key] = val
		}
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "workboard." + t.ID,
		Kind:          kind,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// assureCriteriaMaxTokens bounds the per-criterion verdict reply. It is larger
// than the plain completion check because the judge itemizes each criterion.
const assureCriteriaMaxTokens = 800

// ProveTask closes the proof loop for a task: it runs the acceptance-criteria
// judge over the task's answer, gathers the artifacts + hash-chained journal
// range produced under corr as evidence, records the resulting proof, and
// publishes a proved/unproven event. When the proof is satisfied the task
// reaches done; otherwise it parks in review with the gap visible. A task with
// no acceptance criteria is judged on overall completion alone.
//
// answer is the run output to judge; pass "" to have ProveTask synthesize a
// proxy from the task's latest attempt summary and recent comments (used by the
// manual `agt workboard prove` path, which has no answer in hand).
