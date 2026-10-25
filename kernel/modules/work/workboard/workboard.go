// SPDX-License-Identifier: MIT

// Package workboard is AGEZT's durable typed task queue. It is deliberately
// separate from kernel/board: board is a message mailbox, workboard is a
// restart-safe task state machine that agents and workflows can coordinate on.
//
// The ROWS now live in the contract layer (kernel/contract/workboardapi) —
// W3.31. The module keeps their names as aliases (see aliases.go), so there
// is one row and one spelling of it. The store, the lifecycle transitions
// and the retry rule stay here: they are the implementation.
package workboard

import (
	"errors"
	"sync"
	"time"
)

const (
	storeVersion = 1
	maxTasks     = 10000
)

var (
	ErrClaimConflict = errors.New("workboard: claim conflict")
	ErrNotClaimed    = errors.New("workboard: not claimed")
	ErrClaimFresh    = errors.New("workboard: claim heartbeat is not stale")
	ErrInvalidPolicy = errors.New("workboard: invalid retry policy")
	ErrUnproven      = errors.New("workboard: task has unsatisfied acceptance criteria; prove it before completing")
)

// Store is the durable workboard registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use.
//
// The store is the IMPLEMENTATION: it stays in the module (W3.31 moved the
// rows and the status rule to the contract, not the persistence).
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
