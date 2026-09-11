// SPDX-License-Identifier: MIT

package workboard

// Store more CRUD + internal helpers: Review + SetSeat + Archive + Link +
// AddDependency + BlockingDependencies + ReclaimStale + SweepStaleClaims +
// mutate + find + saveLocked + dependsOnLocked + dependencySatisfied +
// reconcileCriteria + provenGapSummary + finishRunningAttempt. Carved out
// of workboard.go during the Day 36 god file split #1.

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/ulid"
)

func reconcileCriteria(declared, judged []proof.Criterion) []proof.Criterion {
	out := make([]proof.Criterion, len(declared))
	copy(out, declared)
	for i := range out {
		out[i].Met = false
		out[i].Note = ""
		for _, j := range judged {
			if strings.EqualFold(strings.TrimSpace(j.Text), strings.TrimSpace(out[i].Text)) {
				out[i].Met = j.Met
				out[i].Note = j.Note
				break
			}
		}
	}
	return out
}

// provenGapSummary produces a one-line reason a proof failed the gate.
func provenGapSummary(p proof.Proof) string {
	if g := strings.TrimSpace(p.Verdict.Gap); g != "" {
		return g
	}
	if n := p.UnmetCount(); n > 0 {
		return fmt.Sprintf("%d of %d criteria unmet", n, len(p.Criteria))
	}
	return "verifier did not confirm completion"
}

// finishRunningAttempt closes the most recent running attempt with the given
// terminal status (and optional summary), mirroring the inline loops used by
// Complete/Review.
func finishRunningAttempt(t *Task, status string, ts int64, summary string) {
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" {
			t.Attempts[i].Status = status
			t.Attempts[i].FinishedMS = ts
			if summary != "" {
				t.Attempts[i].Summary = summary
			}
			return
		}
	}
}

func (s *Store) Review(id, actor, summary string, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	summary = strings.TrimSpace(summary)
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusReview
		t.Claim = nil
		for i := len(t.Attempts) - 1; i >= 0; i-- {
			if t.Attempts[i].Status == "running" {
				t.Attempts[i].Status = "review"
				t.Attempts[i].FinishedMS = ts
				t.Attempts[i].Summary = summary
				break
			}
		}
		body := "ready for review"
		if summary != "" {
			body += ": " + summary
		}
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
		return nil
	}, now)
}

// SetSeat changes the execution seat a task will be dispatched under. An empty
// seat clears it (back to the agent's defaults). Seat-id validity is enforced at
// the control plane; the store just records the string.
func (s *Store) SetSeat(id, seat string, now time.Time) (Task, error) {
	seat = strings.TrimSpace(seat)
	return s.mutate(id, func(t *Task, _ int64) error {
		t.Seat = seat
		return nil
	}, now)
}

func (s *Store) Archive(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusArchived
		t.ArchivedMS = ts
		t.Claim = nil
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "archived", CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) Link(id, typ, target string, now time.Time) (Task, error) {
	typ = strings.TrimSpace(typ)
	target = strings.TrimSpace(target)
	if typ == "" || target == "" {
		return Task{}, errors.New("workboard: link requires type and target")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Links = append(t.Links, Link{ID: ulid.New(), Type: typ, Target: target, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) AddDependency(id, dependsOn string, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	dependsOn = strings.TrimSpace(dependsOn)
	if id == "" || dependsOn == "" {
		return Task{}, errors.New("workboard: dependency requires id and depends_on")
	}
	if id == dependsOn {
		return Task{}, errors.New("workboard: task cannot depend on itself")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return Task{}, ErrNotFound
	}
	dep := s.find(dependsOn)
	if dep == nil {
		return Task{}, fmt.Errorf("workboard: dependency not found: %s", dependsOn)
	}
	if s.dependsOnLocked(dep.ID, t.ID, map[string]bool{}) {
		return Task{}, errors.New("workboard: dependency would create a cycle")
	}
	prev := cloneTask(*t)
	for _, d := range t.Dependencies {
		if d.ID == dep.ID {
			return cloneTask(*t), nil
		}
	}
	ts := now.UnixMilli()
	t.Dependencies = append(t.Dependencies, Dependency{ID: dep.ID, CreatedMS: ts})
	t.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*t = prev
		return Task{}, err
	}
	return cloneTask(*t), nil
}

func (s *Store) BlockingDependencies(id string) ([]DependencyState, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return nil, ErrNotFound
	}
	var out []DependencyState
	for _, d := range t.Dependencies {
		dep := s.find(d.ID)
		if dep == nil {
			out = append(out, DependencyState{ID: d.ID, Status: "missing", Missing: true, CreatedMS: d.CreatedMS})
			continue
		}
		if dependencySatisfied(*dep) {
			continue
		}
		out = append(out, DependencyState{ID: dep.ID, Title: dep.Title, Status: dep.Status, CreatedMS: d.CreatedMS})
	}
	return out, nil
}

func (s *Store) ReclaimStale(id, actor string, staleAfter time.Duration, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	if staleAfter <= 0 {
		return Task{}, errors.New("workboard: stale_after must be positive")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		return reclaimStaleTask(t, actor, staleAfter, ts)
	}, now)
}

func (s *Store) SweepStaleClaims(actor string, staleAfter time.Duration, limit int, now time.Time) ([]Task, error) {
	actor = strings.TrimSpace(actor)
	if staleAfter <= 0 {
		return nil, errors.New("workboard: stale_after must be positive")
	}
	if limit <= 0 {
		limit = maxTasks
	}
	ts := now.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := make([]Task, len(s.tasks))
	for i, t := range s.tasks {
		prev[i] = cloneTask(*t)
	}
	out := make([]Task, 0)
	for _, t := range s.tasks {
		if len(out) >= limit {
			break
		}
		if t.Claim == nil || ts-t.Claim.HeartbeatMS < staleAfter.Milliseconds() {
			continue
		}
		if err := reclaimStaleTask(t, actor, staleAfter, ts); err != nil {
			continue
		}
		t.UpdatedMS = ts
		out = append(out, cloneTask(*t))
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := s.saveLocked(); err != nil {
		for i := range s.tasks {
			*s.tasks[i] = prev[i]
		}
		return nil, err
	}
	return out, nil
}

func (s *Store) mutate(id string, fn func(*Task, int64) error, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return Task{}, ErrNotFound
	}
	prev := cloneTask(*t)
	ts := now.UnixMilli()
	if err := fn(t, ts); err != nil {
		*t = prev
		return Task{}, err
	}
	t.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*t = prev
		return Task{}, err
	}
	return cloneTask(*t), nil
}

func (s *Store) find(id string) *Task {
	for _, t := range s.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	return jsonstore.Save(s.path, diskState{Version: storeVersion, Tasks: s.tasks})
}

func (s *Store) dependsOnLocked(startID, targetID string, seen map[string]bool) bool {
	if startID == targetID {
		return true
	}
	if seen[startID] {
		return false
	}
	seen[startID] = true
	t := s.find(startID)
	if t == nil {
		return false
	}
	for _, d := range t.Dependencies {
		if s.dependsOnLocked(d.ID, targetID, seen) {
			return true
		}
	}
	return false
}

func dependencySatisfied(t Task) bool {
	return t.Status == StatusDone || t.CompletedMS > 0
}
