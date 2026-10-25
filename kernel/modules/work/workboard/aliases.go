// SPDX-License-Identifier: MIT

package workboard

// The rows and the status rule now live in the contract layer
// (kernel/contract/workboardapi) — W3.31. The module keeps their names as
// ALIASES and its rule as a one-line DELEGATION, so there is one row and one
// spelling of it: the store here, the agent-facing tool there, and the app
// in between all speak the same task.
//
// The store, the lifecycle transitions and the retry rule stay here: they are
// the implementation.

import "github.com/agezt/agezt/kernel/contract/workboardapi"

// The rows.
type (
	// Status is the durable lifecycle state of a workboard task.
	Status = workboardapi.Status
	// Task is one durable work item.
	Task = workboardapi.Task
	// Claim is one agent's hold on a task.
	Claim = workboardapi.Claim
	// Dependency records that a task waits for another one.
	Dependency = workboardapi.Dependency
	// DependencyState is one dependency as the board shows it.
	DependencyState = workboardapi.DependencyState
	// Attempt records one run of a task.
	Attempt = workboardapi.Attempt
	// Comment is one note on a task.
	Comment = workboardapi.Comment
	// Link connects a task to something else.
	Link = workboardapi.Link
	// RetryPolicy is a task's retry budget.
	RetryPolicy = workboardapi.RetryPolicy
	// RetryDecision is the outcome the retry rule reached for a failure.
	RetryDecision = workboardapi.RetryDecision
	// CreateSpec is what an agent hands over to create a task.
	CreateSpec = workboardapi.CreateSpec
	// Filter narrows a listing.
	Filter = workboardapi.Filter
)

// The lifecycle states.
const (
	StatusTriage   = workboardapi.StatusTriage
	StatusTodo     = workboardapi.StatusTodo
	StatusReady    = workboardapi.StatusReady
	StatusRunning  = workboardapi.StatusRunning
	StatusBlocked  = workboardapi.StatusBlocked
	StatusReview   = workboardapi.StatusReview
	StatusDone     = workboardapi.StatusDone
	StatusArchived = workboardapi.StatusArchived
)

// ErrInvalidStatus is returned by ParseStatus for a status the workboard does
// not know.
var ErrInvalidStatus = workboardapi.ErrInvalidStatus

// ErrNotFound is the answer the board gives when a task is not there.
//
// The sentinel lives in the contract layer (kernel/contract/workboardapi) —
// the host is the one who reports it, and the host may not name a module's
// error. The name stays here as an alias, so every existing caller is
// unchanged.
var ErrNotFound = workboardapi.ErrNotFound

// RetryDecisionFor is the retry rule: given a task and the reason it failed,
// what happens next.
//
// The rule lives in the contract layer (kernel/contract/workboardapi) — it is
// pure, and it reads the task, so it travels with it. This is a delegation,
// so there is one rule.
func RetryDecisionFor(t Task, reason string) RetryDecision {
	return workboardapi.RetryDecisionFor(t, reason)
}

// ParseStatus normalizes and validates a status string.
//
// The rule lives in the contract layer (kernel/contract/workboardapi) — it is
// pure, and it is the rule the whole daemon enforces. This is a delegation,
// so there is one rule.
func ParseStatus(s string) (Status, error) { return workboardapi.ParseStatus(s) }

// The store answers the contract's port, so the agent-facing tool can be
// handed the tasks without ever naming this module.
var _ workboardapi.Store = (*Store)(nil)

// The store also answers the contract's HOST port (W3.47) — the twenty verbs
// the host drives the board with. A port change is a compile error here, in
// the module that implements it.
var _ workboardapi.HostStore = (*Store)(nil)
