// SPDX-License-Identifier: MIT

// *Kernel simple getter accessors: Journal, Bus, State, Edict, Warden, Approvals, Scheduler, Provider, Memory, AgentGateway, Schedules, Tools.
// Code extracted from runtime.go during the Day-41 god-file split. Public API unchanged.
package runtime


import (
	"errors"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
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

// Provider exposes the live agent.Provider.
func (k *Kernel) Provider() agent.Provider { return k.cfg.Provider }

// Memory returns the memory-lite manager.
func (k *Kernel) Memory() *memory.Manager { return k.memory }

// AgentGateway returns the Agent Gateway for subprocess communication.
func (k *Kernel) AgentGateway() *agentgw.Gateway { return k.agentGW }

// Schedules returns the persistent typed schedule store.
func (k *Kernel) Schedules() *cadence.Store { return k.schedules }

// Tools returns the live in-process tool map.
func (k *Kernel) Tools() map[string]agent.Tool { return k.tools }

// ErrHalted is returned by Run when the kernel is in halt state.
var ErrHalted = errors.New("runtime: kernel is halted")
