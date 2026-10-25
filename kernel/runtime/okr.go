// SPDX-License-Identifier: MIT

package runtime

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

// errNoObjectiveLibrary is the answer the objective surfaces give when the
// kernel holds no library.
//
// The objective library is the DAEMON's (W3.53): the kernel holds the
// contract's port, and a host that was given none holds a nil port. The
// refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoObjectiveLibrary = errors.New("okr: the objective library is not available on this daemon")

// OKR returns the durable objectives-and-key-results store.
//
// It is the CONTRACT's port, not a module's store (W3.53): the library's file
// is the daemon's business and the kernel may not name the module that keeps
// it. A host that was given no library holds a nil port.
func (k *Kernel) OKR() okrapi.Store { return k.okr }

// taskDone is the rollup signal: a linked workboard task counts toward a Key
// Result once it is done. Because the proof gate only lets a criteria-bearing
// task reach done after its acceptance criteria are proven, this is exactly
// "proven tasks roll up" for gated work; legitimately-completed ungated tasks
// count too.
func (k *Kernel) taskDone(taskID string) bool {
	if k.workboard == nil {
		return false
	}
	t, ok := k.workboard.Get(taskID)
	return ok && t.Status == workboardapi.StatusDone
}

// Rollup computes an objective's progress from the live done-status of its
// linked workboard tasks.
func (k *Kernel) Rollup(o okrapi.Objective) okrapi.ObjectiveProgress {
	return o.Progress(k.taskDone)
}

// ObjectiveRollup returns an objective and its live progress rollup, computed
// from the current done-status of its linked workboard tasks.
func (k *Kernel) ObjectiveRollup(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool) {
	if k.okr == nil {
		return okrapi.Objective{}, okrapi.ObjectiveProgress{}, false
	}
	o, ok := k.okr.Get(id)
	if !ok {
		return okrapi.Objective{}, okrapi.ObjectiveProgress{}, false
	}
	return o, o.Progress(k.taskDone), true
}

func (k *Kernel) CreateObjective(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error) {
	if k.okr == nil {
		return okrapi.Objective{}, errNoObjectiveLibrary
	}
	o, err := k.okr.Create(spec, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	k.publishOKR(corr, event.KindOKRObjectiveCreated, o, "created", nil)
	return o, nil
}

func (k *Kernel) AddObjectiveKeyResult(corr, objID, title string, target int) (okrapi.Objective, error) {
	if k.okr == nil {
		return okrapi.Objective{}, errNoObjectiveLibrary
	}
	o, err := k.okr.AddKeyResult(objID, title, target, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	k.publishOKR(corr, event.KindOKRObjectiveUpdated, o, "key_result_added", map[string]any{"title": title, "target": target})
	return o, nil
}

func (k *Kernel) LinkObjectiveTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	if k.okr == nil {
		return okrapi.Objective{}, errNoObjectiveLibrary
	}
	o, err := k.okr.LinkTask(objID, krID, taskID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	k.publishOKR(corr, event.KindOKRObjectiveUpdated, o, "task_linked", map[string]any{"key_result": krID, "task": taskID})
	// A task linked after it was already done should roll up immediately.
	k.recomputeOKRForTask(corr, taskID)
	return o, nil
}

func (k *Kernel) UnlinkObjectiveTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	if k.okr == nil {
		return okrapi.Objective{}, errNoObjectiveLibrary
	}
	o, err := k.okr.UnlinkTask(objID, krID, taskID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	k.publishOKR(corr, event.KindOKRObjectiveUpdated, o, "task_unlinked", map[string]any{"key_result": krID, "task": taskID})
	return o, nil
}

func (k *Kernel) ArchiveObjective(corr, objID string) (okrapi.Objective, error) {
	if k.okr == nil {
		return okrapi.Objective{}, errNoObjectiveLibrary
	}
	o, err := k.okr.Archive(objID, time.Now().UnixMilli())
	if err != nil {
		return okrapi.Objective{}, err
	}
	k.publishOKR(corr, event.KindOKRObjectiveUpdated, o, "archived", nil)
	return o, nil
}

// recomputeOKRForTask re-rolls every objective that links taskID and flips its
// cached achieved status (emitting an event) when it crosses — or regresses
// below — the target. The rollup itself is always computed live on read; this
// only maintains the durable "achieved" signal + the achieved event that Phase-2
// consumers (and the OKR board) react to. Called after a task is proven or
// completed.
func (k *Kernel) recomputeOKRForTask(corr, taskID string) {
	if k.okr == nil {
		return
	}
	for _, objID := range k.okr.ObjectivesForTask(taskID) {
		o, ok := k.okr.Get(objID)
		if !ok {
			continue
		}
		pr := o.Progress(k.taskDone)
		switch {
		case pr.Achieved && o.Status == okrapi.StatusActive:
			if updated, err := k.okr.SetStatus(objID, okrapi.StatusAchieved, time.Now().UnixMilli()); err == nil {
				k.publishOKR(corr, event.KindOKRObjectiveAchieved, updated, "achieved", map[string]any{"percent": pr.Percent})
			}
		case !pr.Achieved && o.Status == okrapi.StatusAchieved:
			// A linked task reopened after the objective was achieved.
			if updated, err := k.okr.SetStatus(objID, okrapi.StatusActive, time.Now().UnixMilli()); err == nil {
				k.publishOKR(corr, event.KindOKRObjectiveUpdated, updated, "reopened", map[string]any{"percent": pr.Percent})
			}
		}
	}
}

func (k *Kernel) publishOKR(corr string, kind event.Kind, o okrapi.Objective, action string, extra map[string]any) {
	if k.bus == nil {
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
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "okr." + o.ID,
		Kind:          kind,
		Actor:         "okr",
		CorrelationID: corr,
		Payload:       payload,
	})
}
