// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime objectives: the host's names for the objective library.
//
// The library is the WORK MODULE's: the module keeps the store, the goals and
// the rollup rule, and the host holds the module's face
// (kernel/modules/work/api.Service) and forwards to it.
//
// The host's public names did not move — CreateObjective and its siblings are
// still the daemon's verbs, and every caller outside the runtime is
// untouched. What moved is WHERE the answer comes from: the daemon used to
// hold the store and spell out the rollup itself, which left the module
// keeping the DATA and the daemon keeping the OPERATIONS over it.
//
// The composition root builds the module's service over the store it opened
// and hands it over with SetObjectiveLibrary — the same shape the board and
// the reasoning module use. Until it does, the library refuses every verb
// with the contract's row (okrapi.ErrNoLibrary): a daemon with no library
// answers no library.

import (
	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// OKR returns the durable objectives-and-key-results store.
//
// It is the CONTRACT's port, not a module's store (W3.53): the library's file
// is the daemon's business and the kernel may not name the module that keeps
// it. A host that was given no library holds a nil port.
func (k *Kernel) OKR() okrapi.Store { return k.okr }

// SetObjectiveLibrary injects the objective library the daemon drives.
//
// The library is the work module's service: the module keeps the rollup rule,
// the daemon keeps the names. The composition root builds it over the store it
// opened and hands it over here.
func (k *Kernel) SetObjectiveLibrary(s workapi.ObjectiveService) {
	k.configMu.Lock()
	k.objectives = s
	k.configMu.Unlock()
}

// objectiveLibrary returns the injected library, or nil.
func (k *Kernel) objectiveLibrary() workapi.ObjectiveService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.objectives
}

// ObjectiveLibraryDependencies are the host's ports the library needs to
// answer a verb: the bus to publish on, and a task's live status.
//
// The library asks the host; it never reaches into it.
func (k *Kernel) ObjectiveLibraryDependencies() workapi.ObjectiveDependencies {
	return workapi.ObjectiveDependencies{
		Publish: func(spec event.Spec) {
			if k.bus == nil {
				return
			}
			k.bus.Publish(spec)
		},
		TaskStatus: func(taskID string) (workboardapi.Status, bool) {
			if k.workboard == nil {
				return "", false
			}
			t, ok := k.workboard.Get(taskID)
			if !ok {
				return "", false
			}
			return t.Status, true
		},
	}
}

// objectiveLibraryOrRefusal returns the library the daemon drives, or the
// refusal.
//
// The refusal is the contract's row: a daemon with no library answers no
// library, whether the store was never opened or the module was never wired.
func (k *Kernel) objectiveLibraryOrRefusal() (workapi.ObjectiveService, error) {
	svc := k.objectiveLibrary()
	if svc == nil {
		return nil, okrapi.ErrNoLibrary
	}
	return svc, nil
}

// Rollup computes an objective's progress from the live done-status of its
// linked workboard tasks.
func (k *Kernel) Rollup(o okrapi.Objective) okrapi.ObjectiveProgress {
	svc := k.objectiveLibrary()
	if svc == nil {
		// No library, no board, nothing counts — the same answer the daemon
		// gave before the module was wired.
		return o.Progress(func(string) bool { return false })
	}
	return svc.Progress(o)
}

// ObjectiveRollup returns an objective and its live progress rollup, computed
// from the current done-status of its linked workboard tasks.
func (k *Kernel) ObjectiveRollup(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool) {
	svc := k.objectiveLibrary()
	if svc == nil {
		return okrapi.Objective{}, okrapi.ObjectiveProgress{}, false
	}
	return svc.ProgressOf(id)
}

// CreateObjective adds a goal to the library.
func (k *Kernel) CreateObjective(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.Create(corr, spec)
}

// AddObjectiveKeyResult adds a measurable outcome under a goal.
func (k *Kernel) AddObjectiveKeyResult(corr, objID, title string, target int) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.AddKeyResult(corr, objID, title, target)
}

// LinkObjectiveTask counts a task toward a key result.
func (k *Kernel) LinkObjectiveTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.LinkTask(corr, objID, krID, taskID)
}

// UnlinkObjectiveTask stops counting a task.
func (k *Kernel) UnlinkObjectiveTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.UnlinkTask(corr, objID, krID, taskID)
}

// ArchiveObjective retires a goal.
func (k *Kernel) ArchiveObjective(corr, objID string) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.Archive(corr, objID)
}

// recomputeOKRForTask re-rolls every objective that links taskID and flips its
// cached achieved status (emitting an event) when it crosses — or regresses
// below — the target.
//
// It is the daemon's glue: the board's service calls the hook after a task
// reaches done, and the daemon asks the module.
func (k *Kernel) recomputeOKRForTask(corr, taskID string) {
	svc := k.objectiveLibrary()
	if svc == nil {
		return
	}
	svc.RecomputeForTask(corr, taskID)
}
