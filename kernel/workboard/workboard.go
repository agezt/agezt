// SPDX-License-Identifier: MIT

// Package workboard is AGEZT's durable typed task queue. It is deliberately
// separate from kernel/board: board is a message mailbox, workboard is a
// restart-safe task state machine that agents and workflows can coordinate on.
package workboard

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/proof"
)

const (
	storeVersion = 1
	maxTasks     = 10000
)

var (
	ErrNotFound      = errors.New("workboard: not found")
	ErrInvalidStatus = errors.New("workboard: invalid status")
	ErrClaimConflict = errors.New("workboard: claim conflict")
	ErrNotClaimed    = errors.New("workboard: not claimed")
	ErrClaimFresh    = errors.New("workboard: claim heartbeat is not stale")
	ErrInvalidPolicy = errors.New("workboard: invalid retry policy")
	ErrUnproven      = errors.New("workboard: task has unsatisfied acceptance criteria; prove it before completing")
)

// Status is the durable lifecycle state of a workboard task.
type Status string

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
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Description    string            `json:"description,omitempty"`
	Status         Status            `json:"status"`
	Priority       int               `json:"priority,omitempty"`
	Tenant         string            `json:"tenant,omitempty"`
	Assignee       string            `json:"assignee,omitempty"`
	Owner          string            `json:"owner,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Artifacts      []string          `json:"artifacts,omitempty"`
	Criteria       []proof.Criterion `json:"criteria,omitempty"`
	Proof          *proof.Proof      `json:"proof,omitempty"`
	Seat           string            `json:"seat,omitempty"`
	RetryPolicy    *RetryPolicy      `json:"retry_policy,omitempty"`
	Claim          *Claim            `json:"claim,omitempty"`
	Dependencies   []Dependency      `json:"dependencies,omitempty"`
	Attempts       []Attempt         `json:"attempts,omitempty"`
	Comments       []Comment         `json:"comments,omitempty"`
	Links          []Link            `json:"links,omitempty"`
	BlockReason    string            `json:"block_reason,omitempty"`
	CreatedMS      int64             `json:"created_ms"`
	UpdatedMS      int64             `json:"updated_ms"`
	CompletedMS    int64             `json:"completed_ms,omitempty"`
	ArchivedMS     int64             `json:"archived_ms,omitempty"`
}

type Claim struct {
	Agent       string `json:"agent"`
	RunID       string `json:"run_id,omitempty"`
	ClaimedMS   int64  `json:"claimed_ms"`
	HeartbeatMS int64  `json:"heartbeat_ms"`
}

type Dependency struct {
	ID        string `json:"id"`
	CreatedMS int64  `json:"created_ms"`
}

type DependencyState struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    Status `json:"status"`
	Missing   bool   `json:"missing,omitempty"`
	CreatedMS int64  `json:"created_ms,omitempty"`
}

type Attempt struct {
	ID         string `json:"id"`
	Agent      string `json:"agent,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	Status     string `json:"status"`
	StartedMS  int64  `json:"started_ms"`
	FinishedMS int64  `json:"finished_ms,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author,omitempty"`
	Body      string `json:"body"`
	CreatedMS int64  `json:"created_ms"`
}

type Link struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	CreatedMS int64  `json:"created_ms"`
}

type RetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	EscalateTo  string `json:"escalate_to,omitempty"`
}

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
	// kernel/seat). Empty = the assigned agent's defaults. Validation lives at the
	// control plane so this package stays dependency-free.
	Seat        string
	RetryPolicy *RetryPolicy
}

type Filter struct {
	Status          Status
	Tenant          string
	Assignee        string
	IncludeArchived bool
	Limit           int
}

type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	tasks []*Task
}

type diskState struct {
	Version int     `json:"version"`
	Tasks   []*Task `json:"tasks"`
}

// OpenStore opens or creates the workboard store under dir.
