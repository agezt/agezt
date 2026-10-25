// SPDX-License-Identifier: MIT

package okr

// Provenance: OKR: data types (Status + KeyResult + Objective + Progress types) +
//             Objective.Progress method + CreateSpec + Filter + diskState. Code
//             extracted from okr.go during the Day-140 god-file split. Public API
//             unchanged.

import (
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/okrapi"
)

const (
	storeVersion   = 1
	maxObjectives  = 5000
	maxKeyResults  = 50
	maxLinkedTasks = 500
)

// The sentinels the store and the layers above it answer with.
//
// They live in the contract layer (kernel/contract/okrapi); the names stay
// here as aliases, so every existing caller is unchanged.
var (
	ErrNotFound      = okrapi.ErrNotFound
	ErrKRNotFound    = okrapi.ErrKRNotFound
	ErrInvalidStatus = okrapi.ErrInvalidStatus
)

// Status is the lifecycle state of an Objective.
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Status = okrapi.Status

const (
	// StatusActive is a goal in flight.
	StatusActive = okrapi.StatusActive
	// StatusAchieved is a goal whose key results are all met.
	StatusAchieved = okrapi.StatusAchieved
	// StatusArchived is a retired goal, retained for lineage/audit.
	StatusArchived = okrapi.StatusArchived
)

var statusValid = map[Status]bool{StatusActive: true, StatusAchieved: true, StatusArchived: true}

// KeyResult is one measurable outcome under an Objective. Target is the number of
// linked tasks that must be done for the KR to be met; Target == 0 means "all
// linked tasks" (and an empty KR with no tasks is not yet met).
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type KeyResult = okrapi.KeyResult

// Objective is a durable goal owning Key Results.
//
// The row itself lives in the contract layer (kernel/contract/okrapi) — with
// its rollup RULE (Progress) — and the name stays here as an alias, so every
// existing caller is unchanged.
type Objective = okrapi.Objective

// KeyResultProgress is the computed rollup for one Key Result.
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type KeyResultProgress = okrapi.KeyResultProgress

// ObjectiveProgress is the computed rollup for an Objective and its Key Results.
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type ObjectiveProgress = okrapi.ObjectiveProgress

// CreateSpec is the input to Create.
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type CreateSpec = okrapi.CreateSpec

// Filter narrows a listing.
//
// The row itself lives in the contract layer (kernel/contract/okrapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Filter = okrapi.Filter

// Store is the file-backed objective library. It satisfies the contract's
// port, so the layers above it never name this module.
var _ okrapi.Store = (*Store)(nil)

type Store struct {
	path       string
	mu         sync.Mutex
	now        func() time.Time
	objectives []*Objective
}

type diskState struct {
	Version    int          `json:"version"`
	Objectives []*Objective `json:"objectives"`
}

// OpenStore opens or creates the OKR store under dir.
