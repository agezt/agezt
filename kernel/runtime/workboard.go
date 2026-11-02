// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime workboard: the host's face for the task board.
//
// The board is the WORK MODULE's: the module keeps the store, the lifecycle
// and the retry rule.
//
// The daemon used to hold SIXTEEN forwarders over the module's own
// operations — CreateWorkboardTask, ClaimWorkboardTask, CommentWorkboardTask,
// BlockWorkboardTask, FailWorkboardTask, UnblockWorkboardTask,
// SetWorkboardRetryPolicy, CompleteWorkboardTask, ReviewWorkboardTask,
// ArchiveWorkboardTask, LinkWorkboardTask, AddWorkboardDependency,
// ReclaimStaleWorkboardTask, SweepStaleWorkboardClaims, HeartbeatWorkboardTask
// and CreateWorkboardTask's sibling ProveTask. A forwarder over a module's own
// operation is a SECOND SPELLING of it, and the daemon held a second copy of
// the whole board surface for the plugin and the application layer to call
// through.
//
// The board's operations are the module's, so the MODULE implements the
// CONTRACT's runner port over them (kernel/modules/work/workboard/runner.go):
// the contract (workboardapi.Runner) is the surface the agent-facing plugin
// speaks, and its names are the names the board's users say. The module
// answers in them; the daemon no longer spells them at all.
//
// What the daemon keeps is the HOST's own business:
//
//   - the board's FILE (Workboard): the store port the daemon opened;
//   - the board's WIRING (WorkboardDependencies): the host's ports the module
//     asks — the bus it publishes on, the completion judge, the model, the
//     artifact index and the journal;
//   - the module's FACE (BoardRunner): the runner the application layer binds.
//
// The composition root builds the module's service over the store the daemon
// opened and hands the RUNNER over with SetBoardRunner. Until it does, the
// board refuses every verb with the contract's row (workboardapi.ErrNoStore):
// a daemon with no board answers no board.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// Workboard returns the host's board store port.
//
// It is the port the plugins' contract speaks (workboardapi.Store); the
// lifecycle and the proof go through the module's runner.
func (k *Kernel) Workboard() workboardapi.HostStore { return k.workboard }

// SetBoardRunner injects the board's runner — the module's service under the
// CONTRACT's runner port.
//
// The composition root builds it (the module's implementation is not
// importable from the host: a module may import another module's api package
// and nothing else), and hands it over here. Until it does, the board refuses
// every verb with the contract's row: a daemon with no board answers no
// board.
func (k *Kernel) SetBoardRunner(r workapi.BoardRunner) {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	k.boardRunner = r
}

// BoardRunner returns the board's runner — the MODULE's face under the
// contract's port.
//
// It is what the application layer binds (W3.76s): the workboard application
// asks the board, not the daemon. A daemon that was given no runner holds a
// nil face, and the application refuses rather than calling into nothing.
func (k *Kernel) BoardRunner() workapi.BoardRunner {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.boardRunner
}

// WorkboardDependencies returns the host's ports the board needs to answer a
// verb.
//
// The wiring is the HOST's: the bus the board publishes on, the completion
// judge the proof asks, the model the judge speaks with, the artifact index
// and the journal the proof reads. The module holds the board's rules and
// asks the host for these; it never reaches for them.
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
