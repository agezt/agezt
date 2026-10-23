// SPDX-License-Identifier: MIT

package workboardapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/proofapi"
)

// ErrInvalidStatus is returned by ParseStatus for a status the workboard does
// not know.
var ErrInvalidStatus = errors.New("workboard: invalid status")

// Status is the durable lifecycle state of a workboard task.
type Status string

// The lifecycle states.
const (
	StatusTriage   Status = "triage"
	StatusTodo     Status = "todo"
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusBlocked  Status = "blocked"
	StatusReview   Status = "review"
	StatusDone     Status = "done"
	StatusArchived Status = "archived"
)

// statusOrder is the lifecycle's order — it drives the board's columns and
// the sort.
var statusOrder = map[Status]int{
	StatusTriage:   0,
	StatusTodo:     1,
	StatusReady:    2,
	StatusRunning:  3,
	StatusBlocked:  4,
	StatusReview:   5,
	StatusDone:     6,
	StatusArchived: 7,
}

// StatusOrder returns the lifecycle position of a status — it drives the
// board's columns and the sort. A status the workboard does not know has no
// position (-1) — it is not "the first one", which is the triage lane's place.
func StatusOrder(s Status) int {
	pos, ok := statusOrder[s]
	if !ok {
		return -1
	}
	return pos
}

// KnownStatus reports whether the workboard knows a status.
func KnownStatus(s Status) bool {
	_, ok := statusOrder[s]
	return ok
}

// FailedAttemptCount counts the task's failed attempts — the retry rule's
// input, and the number the board shows.
func FailedAttemptCount(t Task) int {
	n := 0
	for _, a := range t.Attempts {
		if attemptCountsAsFailure(a.Status) {
			n++
		}
	}
	return n
}

// attemptCountsAsFailure reports whether an attempt's status counts as a
// failure for the retry budget.
func attemptCountsAsFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stale":
		return true
	}
	return false
}

// ParseStatus normalizes and validates a status string.
func ParseStatus(s string) (Status, error) {
	st := Status(strings.ToLower(strings.TrimSpace(s)))
	if st == "" {
		return "", ErrInvalidStatus
	}
	if _, ok := statusOrder[st]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatus, s)
	}
	return st, nil
}

// Task is one durable work item. Tasks are not agents: an agent may own, claim,
// block, complete, link, or review the task, but the task remains a typed record.
type Task struct {
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	Description    string               `json:"description,omitempty"`
	Status         Status               `json:"status"`
	Priority       int                  `json:"priority,omitempty"`
	Tenant         string               `json:"tenant,omitempty"`
	Assignee       string               `json:"assignee,omitempty"`
	Owner          string               `json:"owner,omitempty"`
	IdempotencyKey string               `json:"idempotency_key,omitempty"`
	Tags           []string             `json:"tags,omitempty"`
	Artifacts      []string             `json:"artifacts,omitempty"`
	Criteria       []proofapi.Criterion `json:"criteria,omitempty"`
	Proof          *proofapi.Proof      `json:"proof,omitempty"`
	Seat           string               `json:"seat,omitempty"`
	RetryPolicy    *RetryPolicy         `json:"retry_policy,omitempty"`
	Claim          *Claim               `json:"claim,omitempty"`
	Dependencies   []Dependency         `json:"dependencies,omitempty"`
	Attempts       []Attempt            `json:"attempts,omitempty"`
	Comments       []Comment            `json:"comments,omitempty"`
	Links          []Link               `json:"links,omitempty"`
	BlockReason    string               `json:"block_reason,omitempty"`
	CreatedMS      int64                `json:"created_ms"`
	UpdatedMS      int64                `json:"updated_ms"`
	CompletedMS    int64                `json:"completed_ms,omitempty"`
	ArchivedMS     int64                `json:"archived_ms,omitempty"`
}

// Claim is one agent's hold on a task.
type Claim struct {
	Agent       string `json:"agent"`
	RunID       string `json:"run_id,omitempty"`
	ClaimedMS   int64  `json:"claimed_ms"`
	HeartbeatMS int64  `json:"heartbeat_ms"`
}

// Dependency records that a task waits for another one.
type Dependency struct {
	ID        string `json:"id"`
	CreatedMS int64  `json:"created_ms"`
}

// DependencyState is one dependency as the board shows it.
type DependencyState struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    Status `json:"status"`
	Missing   bool   `json:"missing,omitempty"`
	CreatedMS int64  `json:"created_ms,omitempty"`
}

// Attempt records one run of a task.
type Attempt struct {
	ID         string `json:"id"`
	Agent      string `json:"agent,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	Status     string `json:"status"`
	StartedMS  int64  `json:"started_ms"`
	FinishedMS int64  `json:"finished_ms,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

// Comment is one note on a task.
type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author,omitempty"`
	Body      string `json:"body"`
	CreatedMS int64  `json:"created_ms"`
}

// Link connects a task to something else.
type Link struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	CreatedMS int64  `json:"created_ms"`
}

// RetryPolicy is a task's retry budget.
type RetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	EscalateTo  string `json:"escalate_to,omitempty"`
}

// RetryDecision is the outcome the retry rule reached for a failure.
type RetryDecision struct {
	Policy       *RetryPolicy `json:"policy,omitempty"`
	FailureCount int          `json:"failure_count"`
	MaxAttempts  int          `json:"max_attempts,omitempty"`
	NextAttempt  int          `json:"next_attempt,omitempty"`
	Retry        bool         `json:"retry"`
	Exhausted    bool         `json:"exhausted"`
	EscalateTo   string       `json:"escalate_to,omitempty"`
	Action       string       `json:"action"`
	Reason       string       `json:"reason,omitempty"`
}

// CreateSpec is what an agent hands over to create a task.
type CreateSpec struct {
	Title          string
	Description    string
	Status         Status
	Priority       int
	Tenant         string
	Assignee       string
	Owner          string
	IdempotencyKey string
	Tags           []string
	Artifacts      []string
	// AcceptanceCriteria are the human-readable conditions the task must satisfy
	// before it may reach done. Declaring any criteria opts the task into the
	// proof gate (see Complete / Prove); leaving it empty keeps the task ungated.
	AcceptanceCriteria []string
	// Seat names the execution preset the task is dispatched under (see
	// kernel/modules/work/seat). Empty = the assigned agent's defaults. Validation lives at the
	// control plane so this package stays dependency-free.
	Seat        string
	RetryPolicy *RetryPolicy
}

// Filter narrows a listing.
type Filter struct {
	Status          Status
	Tenant          string
	Assignee        string
	IncludeArchived bool
	Limit           int
}

// Store is the port through which the stored tasks are read.
//
// The implementation — the on-disk store in the work module — satisfies it;
// the agent-facing tool never names the implementation, because it is a
// plugin.
type Store interface {
	// Get returns one task by id; the bool is false if absent.
	Get(id string) (Task, bool)
	// List returns the tasks that match a filter.
	List(f Filter) []Task
}
