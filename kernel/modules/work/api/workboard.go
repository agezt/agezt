// SPDX-License-Identifier: MIT

package api

// The work module's public vocabulary for the TASK BOARD: the lifecycle
// verbs the host drives, and the host's ports the module needs to answer
// them.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// forwarders, so the host's public API is unchanged while the implementation
// lives in the module.
//
// The ROWS live in the contract layer (kernel/contract/workboardapi): the
// agent-facing tools that expose them are plugins, and a plugin may reach the
// contract and the platform layers only.
//
// Rules of the road:
//
//   - Types, interfaces and ports only. No logic that needs a store, a bus or
//     a clock — those are the implementation package's business.
//   - One row, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

// Service is the task board's public face: the lifecycle the host drives.
//
// The verbs are the BOARD's own, not the daemon's: a host that holds a
// Service can create, claim, complete and archive a task, and it can never
// reach past the board into the daemon's machinery.
//
// Every verb answers with the task it touched, so a caller never reads the
// store to learn what happened.
type Service interface {
	// Create adds a task; the bool is false when the idempotency key was seen.
	Create(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error)
	// Claim takes a task for an agent under a run.
	Claim(corr, id, agent, runID string) (workboardapi.Task, error)
	// Heartbeat keeps a claim alive.
	Heartbeat(corr, id, agent, runID string) (workboardapi.Task, error)
	// Comment adds a note to a task.
	Comment(corr, id, author, body string) (workboardapi.Task, error)
	// Block parks a task with a reason.
	Block(corr, id, actor, reason string) (workboardapi.Task, error)
	// Fail records a failed attempt and returns the retry decision.
	Fail(corr, id, actor, reason string) (workboardapi.Task, workboardapi.RetryDecision, error)
	// Unblock returns a blocked task to the queue.
	Unblock(corr, id, actor string) (workboardapi.Task, error)
	// SetRetryPolicy sets (or clears) a task's retry budget.
	SetRetryPolicy(corr, id, actor string, policy *workboardapi.RetryPolicy) (workboardapi.Task, error)
	// Complete finishes a task.
	Complete(corr, id, actor string) (workboardapi.Task, error)
	// Review moves a task into review.
	Review(corr, id, actor, summary string) (workboardapi.Task, error)
	// Archive retires a task.
	Archive(corr, id, actor string) (workboardapi.Task, error)
	// Link connects a task to something else.
	Link(corr, id, typ, target string) (workboardapi.Task, error)
	// AddDependency makes a task wait for another one.
	AddDependency(corr, id, dependsOn string) (workboardapi.Task, error)
	// ReclaimStale takes a stale claim back.
	ReclaimStale(corr, id, actor string, staleAfter time.Duration) (workboardapi.Task, error)
	// SweepStaleClaims reclaims every stale claim, up to limit.
	SweepStaleClaims(corr, actor string, staleAfter time.Duration, limit int) ([]workboardapi.Task, error)
}

// Dependencies are the host's ports the board needs to answer a verb.
//
// Every port is a way to ASK, never a way to reach: the board publishes on the
// host's bus and tells the host that a task reached done, and it holds
// nothing else of the host's.
type Dependencies struct {
	// Publish emits one event on the host's bus.
	//
	// The host owns the bus; the board only says what happened to a task, and
	// the subject and the kind are the board's own. A nil port means the host
	// has no bus, and the board stays quiet rather than publishing into
	// nothing.
	Publish func(spec event.Spec)

	// TaskCompleted is called after a task reaches done.
	//
	// The objective rollup is the host's business today: a done task counts
	// toward a Key Result, and the host keeps the library. The board calls
	// the hook and holds no knowledge of objectives.
	TaskCompleted func(corr, taskID string)
}
