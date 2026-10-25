// SPDX-License-Identifier: MIT

// Package okrapi is the contract layer's home for the OKR rows: the durable
// goal and its key results, the lifecycle state of a goal, the computed
// rollup, the input that authors one, and the filter that reads them back.
//
// The rows live here because more than one layer has to speak them and none
// of them may name the module's implementation: the kernel creates goals and
// rolls their progress up from the workboard, the app layer reads and curates
// them, and the daemon owns the file they live in. The implementation — the
// file-backed store — stays in the module (kernel/modules/work/okr); the
// module keeps the names its own callers use as aliases, so there is one row
// and one spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package okrapi

import "errors"

// The sentinels the store and the layers above it answer with.
//
// They live in the contract layer because more than one layer has to NAME
// them: the store that answers them and the layers above it that report them
// back to the caller. A module may not be named by the layers above it, so the
// errors the layers above it compare against must not live in the module
// (W3.53).
var (
	// ErrNotFound is returned for operations on an unknown objective id.
	ErrNotFound = errors.New("okr: not found")
	// ErrKRNotFound is returned when a key result id is unknown.
	ErrKRNotFound = errors.New("okr: key result not found")
	// ErrInvalidStatus is returned for a status outside the lifecycle.
	ErrInvalidStatus = errors.New("okr: invalid status")
)

// Status is the lifecycle state of an Objective.
type Status string

const (
	// StatusActive is a goal in flight.
	StatusActive Status = "active"
	// StatusAchieved is a goal whose key results are all met.
	StatusAchieved Status = "achieved"
	// StatusArchived is a retired goal, retained for lineage/audit.
	StatusArchived Status = "archived"
)

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
//
// The RULE travels with the ROW, and that is the point: a Go type alias cannot
// carry methods, so a rule that stayed in the module could not be reached by
// the layers above it — and this rule is what decides whether a goal counts
// as met.
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

// CreateSpec is the input to Create.
type CreateSpec struct {
	Title       string
	Description string
	Owner       string
	Tenant      string
}

// Filter narrows a listing. An empty Status or Tenant matches everything;
// IncludeArchived keeps the retired goals in the answer; Limit caps it.
type Filter struct {
	Status          Status
	Tenant          string
	IncludeArchived bool
	Limit           int
}
