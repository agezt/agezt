// SPDX-License-Identifier: MIT

// OKR persistent Store: OpenStore + every public Store
// method (Create / Get / List / AddKeyResult / LinkTask /
// UnlinkTask / SetStatus / Archive / ObjectivesForTask).
// The private Store methods (mutate / find / saveLocked)
// and the findKR / cloneObjective helpers live in
// okr_store_internals.go. Code extracted from okr.go during
// the Day-140 god-file split. Public API unchanged.
package okr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)


func OpenStore(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	var st diskState
	path, err := jsonstore.LoadFrom(dir, "okr.json", &st)
	if err != nil {
		return nil, fmt.Errorf("okr: %w", err)
	}
	s.path = path
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
