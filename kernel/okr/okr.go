// SPDX-License-Identifier: MIT

// OKR: data types (Status + KeyResult + Objective + Progress types) + Objective.Progress method + CreateSpec + Filter + diskState.
// Code extracted from okr.go during the Day-140 god-file split.
// Public API unchanged.
package okr


import (
	"errors"
	"sync"
	"time"
)


const (
	storeVersion   = 1
	maxObjectives  = 5000
	maxKeyResults  = 50
	maxLinkedTasks = 500
)

var (
	ErrNotFound      = errors.New("okr: not found")
	ErrKRNotFound    = errors.New("okr: key result not found")
	ErrInvalidStatus = errors.New("okr: invalid status")
)

// Status is the lifecycle state of an Objective.
type Status string

const (
	StatusActive   Status = "active"
	StatusAchieved Status = "achieved"
	StatusArchived Status = "archived"
)

var statusValid = map[Status]bool{StatusActive: true, StatusAchieved: true, StatusArchived: true}

// KeyResult is one measurable outcome under an Objective. Target is the number of
// linked tasks that must be done for the KR to be met; Target == 0 means "all
// linked tasks" (and an empty KR with no tasks is not yet met).
type KeyResult struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Target    int      `json:"target,omitempty"`
	TaskIDs   []string `json:"task_ids,omitempty"`
	CreatedMS int64    `json:"created_ms"`
}

// Objective is a durable goal owning Key Results.
type Objective struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Owner       string      `json:"owner,omitempty"`
	Tenant      string      `json:"tenant,omitempty"`
	Status      Status      `json:"status"`
	KeyResults  []KeyResult `json:"key_results,omitempty"`
	CreatedMS   int64       `json:"created_ms"`
	UpdatedMS   int64       `json:"updated_ms"`
	AchievedMS  int64       `json:"achieved_ms,omitempty"`
}

// KeyResultProgress is the computed rollup for one Key Result.
type KeyResultProgress struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Target   int    `json:"target"`
	Percent  int    `json:"percent"`
	Achieved bool   `json:"achieved"`
}

// ObjectiveProgress is the computed rollup for an Objective and its Key Results.
type ObjectiveProgress struct {
	ObjectiveID string              `json:"objective_id"`
	KeyResults  []KeyResultProgress `json:"key_results"`
	Percent     int                 `json:"percent"`
	Achieved    bool                `json:"achieved"`
}

// Progress computes the rollup for the Objective. doneOf reports whether a linked
// task counts as complete (done/proven); the runtime supplies it by consulting
// the workboard. A KR's effective target is Target, or its linked-task count when
// Target is 0. An Objective is achieved when it has at least one Key Result and
// every Key Result is achieved.
func (o Objective) Progress(doneOf func(taskID string) bool) ObjectiveProgress {
	out := ObjectiveProgress{ObjectiveID: o.ID, KeyResults: make([]KeyResultProgress, 0, len(o.KeyResults))}
	allAchieved := len(o.KeyResults) > 0
	sumPct := 0
	for _, kr := range o.KeyResults {
		done := 0
		for _, id := range kr.TaskIDs {
			if doneOf != nil && doneOf(id) {
				done++
			}
		}
		target := kr.Target
		if target <= 0 {
			target = len(kr.TaskIDs)
		}
		pct := 0
		achieved := false
		switch {
		case target <= 0:
			// No tasks and no explicit target: nothing to measure, not achieved.
			pct = 0
		default:
			pct = done * 100 / target
			if pct > 100 {
				pct = 100
			}
			achieved = done >= target
		}
		if !achieved {
			allAchieved = false
		}
		sumPct += pct
		out.KeyResults = append(out.KeyResults, KeyResultProgress{
			ID: kr.ID, Title: kr.Title, Done: done, Total: len(kr.TaskIDs),
			Target: target, Percent: pct, Achieved: achieved,
		})
	}
	if len(out.KeyResults) > 0 {
		out.Percent = sumPct / len(out.KeyResults)
	}
	out.Achieved = allAchieved
	return out
}

type CreateSpec struct {
	Title       string
	Description string
	Owner       string
	Tenant      string
}

type Filter struct {
	Status          Status
	Tenant          string
	IncludeArchived bool
	Limit           int
}

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
