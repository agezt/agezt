// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime workboard: the host's names for the task board.
//
// The board is the WORK MODULE's: the module keeps the store, the lifecycle
// and the retry rule, and the host holds the module's face
// (kernel/modules/work/workapi.BoardService) and forwards to it.
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
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// Workboard returns the host's board store port.
//
// It is the port the plugins' contract speaks (workboardapi.Store); the
// lifecycle and the proof go through the module's service.
func (k *Kernel) Workboard() workboardapi.HostStore { return k.workboard }

// SetWorkboard injects the task board the daemon drives.
//
// The board is the work module's service: the module keeps the lifecycle, the
// daemon keeps the names. The composition root builds it over the store it
// opened and hands it over here.
func (k *Kernel) SetWorkboard(s workapi.BoardService) {
	k.configMu.Lock()
	k.board = s
	k.configMu.Unlock()
}

// boardService returns the injected board, or nil.
func (k *Kernel) boardService() workapi.BoardService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.board
}

// WorkboardDependencies are the host's ports the board needs to answer a
// verb: the bus to publish on, the objective rollup a completed task feeds,
// and the judge's side of the proof cycle.
//
// The board asks the host; it never reaches into it.
//
// The proof's ports (W3.70) are the HOST's machinery: the model that judges an
// answer, the artifacts the answer rests on, the journal the proof points at,
// and the host's own plain completion judge for a task with no criteria. The
// board holds them as ways to ASK — never as a way to reach.
func (k *Kernel) WorkboardDependencies() workapi.BoardDependencies {
	// The journal is a CONCRETE pointer: handing a nil one to an interface
	// would make the port look wired while it is not, and the proof would walk
	// a journal that is not there. The port is nil when the daemon has no
	// journal, and the proof records no journal range.
	var scanner workapi.EventScanner
	if j := k.Journal(); j != nil {
		scanner = j
	}
	return workapi.BoardDependencies{
		Publish: func(spec event.Spec) {
			if k.bus == nil {
				return
			}
			k.bus.Publish(spec)
		},
		TaskCompleted: func(corr, taskID string) {
			k.recomputeOKRForTask(corr, taskID)
		},
		TaskProved: func(corr, taskID string) {
			k.recomputeOKRForTask(corr, taskID)
		},
		Complete: func(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return k.completeAux(ctx, corr, taskType, req)
		},
		Model: func() string {
			return k.Model()
		},
		VerifyCompletion: func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
			return k.VerifyCompletion(ctx, corr, task, answer)
		},
		Artifacts: k.ArtifactIndex(),
		Journal:   scanner,
	}
}

// boardOrRefusal returns the board the daemon drives, or the refusal.
//
// The refusal is the contract's row: a daemon with no board answers no board,
// whether the store was never opened or the module was never wired.
func (k *Kernel) boardOrRefusal() (workapi.BoardService, error) {
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
