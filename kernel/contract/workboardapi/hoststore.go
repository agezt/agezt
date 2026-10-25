// SPDX-License-Identifier: MIT

package workboardapi

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/proofapi"
)

// ErrNotFound is the answer the board gives when a task is not there. It is a
// CONTRACT row, not a module's sentinel: the host is the one who reports it,
// and the host may not name a module's error.
var ErrNotFound = errors.New("workboard: not found")

// HostStore is the port through which the HOST drives the task board: the
// read side (Get, List) plus the lifecycle verbs the host owns.
//
// The port is the host's, and the width is the host's: a plugin holds the
// read side only (`Store`), because a plugin that could claim, complete or
// archive a task would be driving another agent's work item. The host drives
// the whole lifecycle, so the host's port carries all twenty verbs.
//
// The implementation — the on-disk store in the work module — answers it; the
// file it lives in is the host's business, and the kernel that holds this port
// never names the module.
type HostStore interface {
	// Get returns one task by id; the bool is false if absent.
	Get(id string) (Task, bool)
	// List returns the tasks that match a filter.
	List(f Filter) []Task
	// Create adds a task; the bool is false when the idempotency key was seen.
	Create(spec CreateSpec, now time.Time) (Task, bool, error)
	// Claim takes a task for an agent under a run.
	Claim(id, agent, runID string, now time.Time) (Task, error)
	// Heartbeat keeps a claim alive.
	Heartbeat(id, agent, runID string, now time.Time) (Task, error)
	// Comment adds a note to a task.
	Comment(id, author, body string, now time.Time) (Task, error)
	// Block parks a task with a reason.
	Block(id, actor, reason string, now time.Time) (Task, error)
	// Fail records a failed attempt and returns the retry decision.
	Fail(id, actor, reason string, now time.Time) (Task, RetryDecision, error)
	// Unblock returns a blocked task to the queue.
	Unblock(id, actor string, now time.Time) (Task, error)
	// SetRetryPolicy sets (or clears) a task's retry budget.
	SetRetryPolicy(id, actor string, policy *RetryPolicy, now time.Time) (Task, error)
	// Complete finishes a task.
	Complete(id, actor string, now time.Time) (Task, error)
	// Prove records the proof that closes a task's acceptance criteria.
	Prove(id, actor string, p proofapi.Proof, now time.Time) (Task, error)
	// Review moves a task into review.
	Review(id, actor, summary string, now time.Time) (Task, error)
	// SetSeat binds a task to an execution seat.
	SetSeat(id, seat string, now time.Time) (Task, error)
	// Archive retires a task.
	Archive(id, actor string, now time.Time) (Task, error)
	// Link connects a task to something else.
	Link(id, typ, target string, now time.Time) (Task, error)
	// AddDependency makes a task wait for another one.
	AddDependency(id, dependsOn string, now time.Time) (Task, error)
	// BlockingDependencies returns the dependencies that still block a task.
	BlockingDependencies(id string) ([]DependencyState, error)
	// ReclaimStale takes a stale claim back.
	ReclaimStale(id, actor string, staleAfter time.Duration, now time.Time) (Task, error)
	// SweepStaleClaims reclaims every stale claim, up to limit.
	SweepStaleClaims(actor string, staleAfter time.Duration, limit int, now time.Time) ([]Task, error)
}
