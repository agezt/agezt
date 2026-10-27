// SPDX-License-Identifier: MIT

package runtime

// Provenance: *Kernel simple getter accessors: Journal, Bus, State, Edict, Warden,
//             Approvals, Scheduler, Provider, Memory, Schedules,
//             Tools. Code extracted from runtime.go during the Day-41 god-file
//             split. Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime/runexec"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
)

// Journal exposes the underlying journal for read-only inspection.
func (k *Kernel) Journal() *journal.Journal { return k.journal }

// Bus exposes the underlying bus for the control plane to attach subscribers.
func (k *Kernel) Bus() *bus.Bus { return k.bus }

// State returns the durable key/value store backing the kernel.
func (k *Kernel) State() *state.FileStore { return k.state }

// Edict exposes the policy engine for read/configure.
func (k *Kernel) Edict() *edict.Engine { return k.edict }

// Warden exposes the isolation engine.
func (k *Kernel) Warden() warden.Engine { return k.warden }

// Approvals exposes the HITL queue.
func (k *Kernel) Approvals() *approval.Registry { return k.approvals }

// Scheduler exposes the DAG executor.
func (k *Kernel) Scheduler() *scheduler.Executor { return k.scheduler }

// Provider exposes the live llm.Provider.
func (k *Kernel) Provider() llm.Provider { return k.cfg.Provider }

// Memory returns the memory-lite manager — the contract's port (W3.57), not the
// module's concrete manager. A host that was given no memory holds a NIL port.
func (k *Kernel) Memory() memoryapi.Manager { return k.memory }

// Schedules returns the persistent typed schedule store — the contract's HOST
// port (W3.49), not the module's concrete store. The library's file is the
// daemon's; the kernel holds the port and drives the schedule lifecycle
// through it.
func (k *Kernel) Schedules() cadenceapi.HostStore { return k.schedules }

// Tools returns the live in-process tool map.
func (k *Kernel) Tools() map[string]toolapi.Tool { return k.tools }

// ErrHalted is returned by Run when the kernel is in halt state. It is
// runexec's sentinel re-exported (one value, one identity).
var ErrHalted = runexec.ErrHalted
