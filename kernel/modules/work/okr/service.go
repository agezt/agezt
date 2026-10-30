// SPDX-License-Identifier: MIT

package okr

// The library's SERVICE: the objective verbs, over the contract's store port.
//
// The verbs used to live in the daemon (kernel/runtime/okr.go): the daemon
// held the library, called the store and published the event. The DATA was
// already here — the store and the durable library are the module's
// implementation — so the daemon was the only place that knew what a goal's
// PROGRESS meant.
//
// The service closes that split: the module that keeps the library also keeps
// the operations over it, and the daemon holds the module's face
// (kernel/modules/work/api.Service) and forwards to it.
//
// The store port stays the CONTRACT's (okrapi.Store): the file it lives in is
// the host's business, and the host may not name the module that keeps it.
// The service is built over that port by the composition root, which is the
// one place that knows both.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

// service is the library's own face over the host's store port.
type service struct {
	store okrapi.Store
	deps  api.ObjectiveDependencies
}

// NewService builds the library's service over the host's store port.
//
// A nil store is not an error here: the library REFUSES every verb, and the
// refusal is the contract's row (okrapi.ErrNoLibrary) — the same answer the
// daemon gives when it holds no library.
func NewService(store okrapi.Store, deps api.ObjectiveDependencies) api.ObjectiveService {
	return &service{store: store, deps: deps}
}

// storeOrRefusal returns the store, or the contract's refusal.
func (s *service) storeOrRefusal() (okrapi.Store, error) {
	if s.store == nil {
		return nil, okrapi.ErrNoLibrary
	}
	return s.store, nil
}

// taskDone is the rollup signal: a linked workboard task counts toward a Key
// Result once it is done.
//
// Because the proof gate only lets a criteria-bearing task reach done after
// its acceptance criteria are proven, this is exactly "proven tasks roll up"
// for gated work; legitimately-completed ungated tasks count too.
func (s *service) taskDone(taskID string) bool {
	if s.deps.TaskStatus == nil {
		return false
	}
	status, ok := s.deps.TaskStatus(taskID)
	return ok && status == workboardapi.StatusDone
}

// Progress computes a goal's progress from the live done-status of its linked
// workboard tasks.
func (s *service) Progress(o okrapi.Objective) okrapi.ObjectiveProgress {
	return o.Progress(s.taskDone)
}

// ProgressOf returns a goal and its live progress rollup, computed from the
// current done-status of its linked workboard tasks.
func (s *service) ProgressOf(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, okrapi.ObjectiveProgress{}, false
	}
	o, ok := store.Get(id)
	if !ok {
		return okrapi.Objective{}, okrapi.ObjectiveProgress{}, false
	}
	return o, o.Progress(s.taskDone), true
}

func (s *service) Create(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	o, err := store.Create(spec, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	s.publish(corr, event.KindOKRObjectiveCreated, o, "created", nil)
	return o, nil
}

func (s *service) AddKeyResult(corr, objID, title string, target int) (okrapi.Objective, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	o, err := store.AddKeyResult(objID, title, target, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	s.publish(corr, event.KindOKRObjectiveUpdated, o, "key_result_added", map[string]any{"title": title, "target": target})
	return o, nil
}

func (s *service) LinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	o, err := store.LinkTask(objID, krID, taskID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	s.publish(corr, event.KindOKRObjectiveUpdated, o, "task_linked", map[string]any{"key_result": krID, "task": taskID})
	// A task linked after it was already done should roll up immediately.
	s.RecomputeForTask(corr, taskID)
	return o, nil
}

func (s *service) UnlinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	o, err := store.UnlinkTask(objID, krID, taskID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	s.publish(corr, event.KindOKRObjectiveUpdated, o, "task_unlinked", map[string]any{"key_result": krID, "task": taskID})
	return o, nil
}

func (s *service) Archive(corr, objID string) (okrapi.Objective, error) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	o, err := store.Archive(objID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	s.publish(corr, event.KindOKRObjectiveUpdated, o, "archived", nil)
	return o, nil
}

// RecomputeForTask re-rolls every goal that links taskID and flips its cached
// achieved status (emitting an event) when it crosses — or regresses below —
// the target. The rollup itself is always computed live on read; this only
// maintains the durable "achieved" signal + the achieved event that Phase-2
// consumers (and the OKR board) react to. Called after a task is proven or
// completed.
func (s *service) RecomputeForTask(corr, taskID string) {
	store, err := s.storeOrRefusal()
	if err != nil {
		return
	}
	for _, objID := range store.ObjectivesForTask(taskID) {
		o, ok := store.Get(objID)
		if !ok {
			continue
		}
		pr := o.Progress(s.taskDone)
		switch {
		case pr.Achieved && o.Status == okrapi.StatusActive:
			if updated, err := store.SetStatus(objID, okrapi.StatusAchieved, time.Now().UnixMilli()); err == nil {
				s.publish(corr, event.KindOKRObjectiveAchieved, updated, "achieved", map[string]any{"percent": pr.Percent})
			}
		case !pr.Achieved && o.Status == okrapi.StatusAchieved:
			// A linked task reopened after the goal was achieved.
			if updated, err := store.SetStatus(objID, okrapi.StatusActive, time.Now().UnixMilli()); err == nil {
				s.publish(corr, event.KindOKRObjectiveUpdated, updated, "reopened", map[string]any{"percent": pr.Percent})
			}
		}
	}
}

// publish says what happened to a goal on the host's bus.
//
// The subject, the kind and the payload are the LIBRARY's own: the host's
// port carries the event, the library decides what it says. A host with no
// bus (no publish port) is quiet rather than broken — the goal still moved,
// and the caller still holds the answer.
func (s *service) publish(corr string, kind event.Kind, o okrapi.Objective, action string, extra map[string]any) {
	if s.deps.Publish == nil {
		return
	}
	payload := map[string]any{
		"id":     o.ID,
		"title":  o.Title,
		"status": string(o.Status),
		"action": action,
	}
	for key, val := range extra {
		if val != "" {
			payload[key] = val
		}
	}
	s.deps.Publish(event.Spec{
		Subject:       "okr." + o.ID,
		Kind:          kind,
		Actor:         "okr",
		CorrelationID: corr,
		Payload:       payload,
	})
}
