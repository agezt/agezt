// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime objectives: the host's face for the objective library.
//
// The library is the WORK MODULE's: the module keeps the store, the goals and
// the rollup rule, and the host holds the module's face
// (kernel/modules/work/api.ObjectiveService).
//
// The host's public names are GONE (W3.76r). CreateObjective,
// AddObjectiveKeyResult, LinkObjectiveTask, UnlinkObjectiveTask,
// ArchiveObjective and Rollup used to be the daemon's verbs — six thin
// forwarders over the module's own service. A forwarder over a module's own
// operation is a second spelling, and the application layer was paying for it:
// the app's objective ports were bound to the DAEMON, so the objective
// surface reached the module through the host on every call.
//
// Now the application binds the MODULE: kernel/app/okr holds the work module's
// ObjectiveService, and it does not import this package at all. The daemon
// keeps two things and no more:
//
//   - the STORE PORT (OKR): the library's file is the daemon's business
//     (W3.53), and the contract's store port is the host's face over it;
//   - the SERVICE (Objectives): the module's face, so the composition root's
//     scope can hand it to the application.
//
// The composition root builds the service over the store the daemon opened and
// the daemon's own ports, through the BUILDER in runtime.Config
// (ObjectiveLibrary) — the same shape the roster, the MCP registry and the
// script forge use. Until it does, the library refuses every verb with the
// contract's row (okrapi.ErrNoLibrary): a daemon with no library answers no
// library.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/event"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// OKR returns the durable objectives-and-key-results store.
//
// It is the CONTRACT's port, not a module's store (W3.53): the library's file
// is the daemon's business and the kernel may not name the module that keeps
// it. A host that was given no library holds a nil port.
func (k *Kernel) OKR() okrapi.Store { return k.okr }

// Objectives returns the work module's objective library service.
//
// It is the MODULE's face, and it is what the application layer binds (W3.76r):
// the objective surface asks the module, not the daemon. A daemon that was
// given no builder holds a nil service, and the objective surfaces report that
// the library is unavailable rather than calling into nothing.
func (k *Kernel) Objectives() workapi.ObjectiveService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.objectives
}

// objectiveLibrary returns the injected library, or nil.
func (k *Kernel) objectiveLibrary() workapi.ObjectiveService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.objectives
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

// objectivePublisher is the host's bus seen from the objective library: the
// module publishes one event and the host drops the reply.
//
// The bus is the HOST's; the library only says what happened to a goal, so
// the daemon hands over a way to ASK it — the same shape the roster and the
// MCP registry use.
type objectivePublisher struct{ b *bus.Bus }

// Publish emits one event on the host's bus, when the host has one.
func (p objectivePublisher) Publish(spec event.Spec) {
	if p.b == nil {
		return
	}
	_, _ = p.b.Publish(spec)
}
