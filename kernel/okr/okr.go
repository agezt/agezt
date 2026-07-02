// SPDX-License-Identifier: MIT

// Package okr is AGEZT's durable objectives-and-key-results spine: the layer
// that makes fleet work legible as progress toward goals rather than a flat task
// queue. An Objective owns Key Results; each Key Result links workboard tasks and
// rolls their completion up into a percentage.
//
// The rollup is deliberately fed by DONE tasks, and because the proof gate
// (kernel/proof + kernel/workboard) only lets a criteria-bearing task reach done
// once its acceptance criteria are proven, "done tasks rolling up" is exactly
// "proven tasks rolling up" for gated work — legitimately-completed ungated tasks
// count too. This package stays pure data (no kernel deps): the runtime supplies
// the task-status closure, so Progress is trivially testable.
package okr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
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
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("okr: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "okr.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("okr: read %s: %w", s.path, err)
	}
	if len(b) == 0 {
		return s, nil
	}
	var st diskState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("okr: parse %s: %w", s.path, err)
	}
	for _, o := range st.Objectives {
		if o == nil {
			continue
		}
		if !statusValid[o.Status] {
			return nil, fmt.Errorf("%w in %s: %s", ErrInvalidStatus, s.path, o.Status)
		}
		cp := cloneObjective(*o)
		s.objectives = append(s.objectives, &cp)
	}
	return s, nil
}

// Create inserts a new Objective.
func (s *Store) Create(spec CreateSpec, now time.Time) (Objective, error) {
	spec.Title = strings.TrimSpace(spec.Title)
	if spec.Title == "" {
		return Objective{}, errors.New("okr: title required")
	}
	ts := now.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.objectives) >= maxObjectives {
		return Objective{}, fmt.Errorf("okr: at most %d objectives", maxObjectives)
	}
	o := Objective{
		ID:          ulid.New(),
		Title:       spec.Title,
		Description: strings.TrimSpace(spec.Description),
		Owner:       strings.TrimSpace(spec.Owner),
		Tenant:      strings.TrimSpace(spec.Tenant),
		Status:      StatusActive,
		CreatedMS:   ts,
		UpdatedMS:   ts,
	}
	s.objectives = append(s.objectives, &o)
	if err := s.saveLocked(); err != nil {
		s.objectives = s.objectives[:len(s.objectives)-1]
		return Objective{}, err
	}
	return cloneObjective(o), nil
}

func (s *Store) Get(id string) (Objective, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o := s.find(id); o != nil {
		return cloneObjective(*o), true
	}
	return Objective{}, false
}

func (s *Store) List(f Filter) []Objective {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Objective, 0, len(s.objectives))
	for _, o := range s.objectives {
		if !f.IncludeArchived && o.Status == StatusArchived {
			continue
		}
		if f.Status != "" && o.Status != f.Status {
			continue
		}
		if f.Tenant != "" && o.Tenant != f.Tenant {
			continue
		}
		out = append(out, cloneObjective(*o))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ia, ja := out[i].Status == StatusArchived, out[j].Status == StatusArchived
		if ia != ja {
			return !ia
		}
		if out[i].UpdatedMS != out[j].UpdatedMS {
			return out[i].UpdatedMS > out[j].UpdatedMS
		}
		return out[i].ID < out[j].ID
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// AddKeyResult appends a Key Result to an Objective.
func (s *Store) AddKeyResult(objID, title string, target int, now time.Time) (Objective, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Objective{}, errors.New("okr: key result title required")
	}
	if target < 0 {
		return Objective{}, errors.New("okr: target must be non-negative")
	}
	return s.mutate(objID, func(o *Objective, ts int64) error {
		if len(o.KeyResults) >= maxKeyResults {
			return fmt.Errorf("okr: at most %d key results", maxKeyResults)
		}
		o.KeyResults = append(o.KeyResults, KeyResult{ID: ulid.New(), Title: title, Target: target, CreatedMS: ts})
		return nil
	}, now)
}

// LinkTask attaches a workboard task id to a Key Result (idempotent).
func (s *Store) LinkTask(objID, krID, taskID string, now time.Time) (Objective, error) {
	krID = strings.TrimSpace(krID)
	taskID = strings.TrimSpace(taskID)
	if krID == "" || taskID == "" {
		return Objective{}, errors.New("okr: link requires key result id and task id")
	}
	return s.mutate(objID, func(o *Objective, _ int64) error {
		kr := findKR(o, krID)
		if kr == nil {
			return ErrKRNotFound
		}
		for _, id := range kr.TaskIDs {
			if id == taskID {
				return nil
			}
		}
		if len(kr.TaskIDs) >= maxLinkedTasks {
			return fmt.Errorf("okr: at most %d linked tasks per key result", maxLinkedTasks)
		}
		kr.TaskIDs = append(kr.TaskIDs, taskID)
		return nil
	}, now)
}

// UnlinkTask removes a task id from a Key Result (idempotent).
func (s *Store) UnlinkTask(objID, krID, taskID string, now time.Time) (Objective, error) {
	krID = strings.TrimSpace(krID)
	taskID = strings.TrimSpace(taskID)
	return s.mutate(objID, func(o *Objective, _ int64) error {
		kr := findKR(o, krID)
		if kr == nil {
			return ErrKRNotFound
		}
		out := kr.TaskIDs[:0]
		for _, id := range kr.TaskIDs {
			if id != taskID {
				out = append(out, id)
			}
		}
		kr.TaskIDs = append([]string(nil), out...)
		return nil
	}, now)
}

// SetStatus transitions an Objective's status (achieved/active/archived). Setting
// achieved stamps AchievedMS; leaving achieved clears it.
func (s *Store) SetStatus(id string, status Status, now time.Time) (Objective, error) {
	if !statusValid[status] {
		return Objective{}, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}
	return s.mutate(id, func(o *Objective, ts int64) error {
		o.Status = status
		if status == StatusAchieved {
			if o.AchievedMS == 0 {
				o.AchievedMS = ts
			}
		} else {
			o.AchievedMS = 0
		}
		return nil
	}, now)
}

// Archive marks an Objective archived.
func (s *Store) Archive(id string, now time.Time) (Objective, error) {
	return s.SetStatus(id, StatusArchived, now)
}

// ObjectivesForTask returns the ids of objectives that link taskID under any Key
// Result — used to recompute rollups when a task is proven.
func (s *Store) ObjectivesForTask(taskID string) []string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, o := range s.objectives {
		if o.Status == StatusArchived {
			continue
		}
		for _, kr := range o.KeyResults {
			linked := false
			for _, id := range kr.TaskIDs {
				if id == taskID {
					linked = true
					break
				}
			}
			if linked {
				out = append(out, o.ID)
				break
			}
		}
	}
	return out
}

func (s *Store) mutate(id string, fn func(*Objective, int64) error, now time.Time) (Objective, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Objective{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.find(id)
	if o == nil {
		return Objective{}, ErrNotFound
	}
	prev := cloneObjective(*o)
	ts := now.UnixMilli()
	if err := fn(o, ts); err != nil {
		*o = prev
		return Objective{}, err
	}
	o.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*o = prev
		return Objective{}, err
	}
	return cloneObjective(*o), nil
}

func (s *Store) find(id string) *Objective {
	for _, o := range s.objectives {
		if o.ID == id {
			return o
		}
	}
	return nil
}

func findKR(o *Objective, krID string) *KeyResult {
	for i := range o.KeyResults {
		if o.KeyResults[i].ID == krID {
			return &o.KeyResults[i]
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(diskState{Version: storeVersion, Objectives: s.objectives}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if stdruntime.GOOS == "windows" {
			if removeErr := os.Remove(s.path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmp, s.path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func cloneObjective(o Objective) Objective {
	krs := make([]KeyResult, len(o.KeyResults))
	for i, kr := range o.KeyResults {
		kr.TaskIDs = append([]string(nil), kr.TaskIDs...)
		krs[i] = kr
	}
	o.KeyResults = krs
	return o
}
