// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime workboard: the host's names for the task board.
//
// The board is the WORK MODULE's: the module keeps the store, the lifecycle
// and the retry rule, and the host holds the module's face
// (kernel/modules/work/workapi.Service) and forwards to it.
//
// The host's public names did not move — ClaimWorkboardTask and its fifteen
// siblings are still the daemon's verbs, and every caller outside the runtime
// is untouched. What moved is WHERE the answer comes from: the daemon used
// to hold the store and spell out the lifecycle itself, which left the
// module keeping the DATA and the daemon keeping the OPERATIONS over it.
//
// The composition root builds the module's service over the store it opened
// and hands it over with SetWorkboard — the same shape the reasoning module
// uses. Until it does, the board refuses every verb with the contract's row
// (workboardapi.ErrNoStore): a daemon with no board answers no board.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// workboardStore returns the host's board store port, or the refusal.
//
// The store file is the HOST's business (W3.47): the daemon opens it and
// hands the port to the module. The proof path still drives the store
// directly; the lifecycle verbs go through the module's service.
func (k *Kernel) workboardStore() (workboardapi.HostStore, error) {
	if k.workboard == nil {
		return nil, workboardapi.ErrNoStore
	}
	return k.workboard, nil
}

// Workboard returns the host's board store port.
//
// It is the port the plugins' contract speaks (workboardapi.Store); the
// lifecycle the daemon drives goes through the module's service.
func (k *Kernel) Workboard() workboardapi.HostStore { return k.workboard }

// SetWorkboard injects the task board the daemon drives.
//
// The board is the work module's service: the module keeps the lifecycle, the
// daemon keeps the names. The composition root builds it over the store it
// opened and hands it over here.
func (k *Kernel) SetWorkboard(s workapi.Service) {
	k.configMu.Lock()
	k.board = s
	k.configMu.Unlock()
}

// boardService returns the injected board, or nil.
func (k *Kernel) boardService() workapi.Service {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.board
}

// WorkboardDependencies are the host's ports the board needs to answer a
// verb: the bus to publish on, and the objective rollup a completed task
// feeds.
//
// The board asks the host; it never reaches into it.
func (k *Kernel) WorkboardDependencies() workapi.Dependencies {
	return workapi.Dependencies{
		Publish: func(spec event.Spec) {
			if k.bus == nil {
				return
			}
			k.bus.Publish(spec)
		},
		TaskCompleted: func(corr, taskID string) {
			k.recomputeOKRForTask(corr, taskID)
		},
	}
}

// boardOrRefusal returns the board the daemon drives, or the refusal.
//
// The refusal is the contract's row: a daemon with no board answers no board,
// whether the store was never opened or the module was never wired.
func (k *Kernel) boardOrRefusal() (workapi.Service, error) {
	svc := k.boardService()
	if svc == nil {
		return nil, workboardapi.ErrNoStore
	}
	return svc, nil
}

// CreateWorkboardTask adds a task to the board.
func (k *Kernel) CreateWorkboardTask(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, false, err
	}
	return svc.Create(corr, spec)
}

// ClaimWorkboardTask takes a task for an agent under a run.
func (k *Kernel) ClaimWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Claim(corr, id, agent, runID)
}

// HeartbeatWorkboardTask keeps a claim alive.
func (k *Kernel) HeartbeatWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Heartbeat(corr, id, agent, runID)
}

// CommentWorkboardTask adds a note to a task.
func (k *Kernel) CommentWorkboardTask(corr, id, author, body string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Comment(corr, id, author, body)
}

// BlockWorkboardTask parks a task with a reason.
func (k *Kernel) BlockWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Block(corr, id, actor, reason)
}

// FailWorkboardTask records a failed attempt and returns the retry decision.
func (k *Kernel) FailWorkboardTask(corr, id, actor, reason string) (workboardapi.Task, workboardapi.RetryDecision, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, workboardapi.RetryDecision{}, err
	}
	return svc.Fail(corr, id, actor, reason)
}

// UnblockWorkboardTask returns a blocked task to the queue.
func (k *Kernel) UnblockWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Unblock(corr, id, actor)
}

// SetWorkboardRetryPolicy sets (or clears) a task's retry budget.
func (k *Kernel) SetWorkboardRetryPolicy(corr, id, actor string, policy *workboardapi.RetryPolicy) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.SetRetryPolicy(corr, id, actor, policy)
}

// CompleteWorkboardTask finishes a task.
func (k *Kernel) CompleteWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Complete(corr, id, actor)
}

// ReviewWorkboardTask moves a task into review.
func (k *Kernel) ReviewWorkboardTask(corr, id, actor, summary string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Review(corr, id, actor, summary)
}

// ArchiveWorkboardTask retires a task.
func (k *Kernel) ArchiveWorkboardTask(corr, id, actor string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Archive(corr, id, actor)
}

// LinkWorkboardTask connects a task to something else.
func (k *Kernel) LinkWorkboardTask(corr, id, typ, target string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Link(corr, id, typ, target)
}

// AddWorkboardDependency makes a task wait for another one.
func (k *Kernel) AddWorkboardDependency(corr, id, dependsOn string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.AddDependency(corr, id, dependsOn)
}

// ReclaimStaleWorkboardTask takes a stale claim back.
func (k *Kernel) ReclaimStaleWorkboardTask(corr, id, actor string, staleAfter time.Duration) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.ReclaimStale(corr, id, actor, staleAfter)
}

// SweepStaleWorkboardClaims reclaims every stale claim, up to limit.
func (k *Kernel) SweepStaleWorkboardClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return nil, err
	}
	return svc.SweepStaleClaims(corr, actor, staleAfter, limit)
}

// publishWorkboard says what happened to a task on the daemon's bus.
//
// It is the PROOF path's publisher: the proof loop still lives in the daemon
// (it judges with the daemon's model), so it publishes with the same payload
// the module's service publishes — the contract's row
// (workboardapi.TaskEventPayload), because the tools read one shape.
//
// A daemon with no bus is quiet rather than broken.
func (k *Kernel) publishWorkboard(corr string, kind event.Kind, t workboardapi.Task, action string, extra map[string]any) {
	if k.bus == nil {
		return
	}
	k.bus.Publish(event.Spec{
		Subject:       "workboard." + t.ID,
		Kind:          kind,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       workboardapi.TaskEventPayload(t, action, extra),
	})
}
