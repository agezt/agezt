// SPDX-License-Identifier: MIT

package system

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/types"
	journalstore "github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
)

// The daemon overview's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The overview is the daemon's own report about
// itself: what it is running, what it holds, what it is waiting for — and it
// reads **eight** families to answer (the journal, the schedules, the
// approvals, the world, the skill library, the tools, the memory and the
// default identity) plus the daemon's own state.
//
// The overview is a READER, and a reader declares its OWN narrow view over
// what it reads: the families already narrow the journal between them (the
// roster's range, the channel inbox's append, the pulse stream's tail), and
// the overview's own narrow views belong to the overview. So the port carries
// the thirteen reads the overview makes, and nothing else: the port is the
// overview's list of questions, not a second handle.
//
// The port holds FUNCTIONS, not values: a question is asked when it is asked,
// and a daemon that grows a capability does not grow the port.
//
// The handle may be nil: a host that holds no daemon holds no daemon to
// report on, and the port says so instead of answering with a lie.

var (
	// ErrNoDaemon is the answer a host with no daemon gives.
	//
	// The overview reports on a daemon. A host with none has nothing to
	// report, and a status full of zeros would read as "the daemon has no
	// tools and no memory" — a claim about a daemon that does not exist.
	// The refusal is the honest answer, and it is written down **once**,
	// here.
	ErrNoDaemon = errors.New("system: no daemon on this host")

	// ErrNoGate is the answer a host with no daemon gives when the daemon's
	// own gate is asked for.
	//
	// The gate is the halt, the resume and the hash-chain check. A host with
	// no daemon holds none of them, and a service that halted nothing while
	// answering "halted" would be lying about a daemon that does not exist.
	ErrNoGate = errors.New("system: no daemon gate on this host")
)

// Gate is the daemon's own gate: the halt that stops new runs, the resume
// that lets them start again, and the journal's hash-chain check.
//
// It is a CAPABILITY, not the handle: the daemon's own lifecycle service
// needs these three doors and nothing else, and a service that holds the
// handle to reach three verbs holds the whole kernel.
type Gate interface {
	HaltWith(reason string)
	ResumeWith(reason string)
	Verify() error
}

// Ports are the daemon overview's capabilities.
//
// Every capability is held UNEXPORTED: the daemon may be absent, and the
// port answers for that case instead of handing the caller a nil object to
// call into.
type Ports struct {
	events     func() *journalstore.Journal
	started    func() time.Time
	delegation func() types.SubAgentLimits
	entries    func() []cadenceapi.Entry
	running    func() (int, bool)
	pending    func() int
	entities   func() int
	skills     func() int
	model      func() string
	stopped    func() bool
	active     func() int
	tools      func() int
	records    func() int
	gate       func() Gate
}

// PortsFromKernel fills the daemon overview's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to
// report on, and the ports answer for that case.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		events:  k.Journal,
		started: k.StartTime,
		delegation: func() types.SubAgentLimits {
			return k.SubAgentLimits()
		},
		entries: func() []cadenceapi.Entry {
			store := k.Schedules()
			if store == nil {
				return nil
			}
			return store.List()
		},
		running: func() (int, bool) {
			engine := k.ScheduleEngine()
			if engine == nil {
				return 0, false
			}
			return engine.RunningCount(), true
		},
		pending: func() int {
			registry := k.Approvals()
			if registry == nil {
				return 0
			}
			return registry.PendingCount()
		},
		entities: func() int {
			graph := k.World()
			if graph == nil {
				return 0
			}
			return graph.Count()
		},
		skills: func() int {
			curator := k.Forge()
			if curator == nil {
				return 0
			}
			return curator.Count()
		},
		model:   k.Model,
		stopped: k.IsHalted,
		active:  k.ActiveRuns,
		tools: func() int {
			return len(k.Tools())
		},
		records: k.MemoryCount,
		gate:    func() Gate { return k },
	}
}

// Ready reports whether the ports hold a daemon to report on.
func (p Ports) Ready() bool { return p.events != nil }

// Events is the journal the overview folds into its fallback counts.
//
// It MAY be absent: the daemon may hold no journal, and the overview then
// reports no fallbacks rather than reading from nothing.
func (p Ports) Events() *journalstore.Journal {
	if p.events == nil {
		return nil
	}
	return p.events()
}

// Started is the moment the daemon started.
//
// It is the zero time when the daemon is absent, and the overview does not
// report an uptime for a daemon it does not have.
func (p Ports) Started() time.Time {
	if p.started == nil {
		return time.Time{}
	}
	return p.started()
}

// Delegation reports the daemon's delegation-governance ceilings: the depth,
// the fan-out and the spend.
func (p Ports) Delegation() types.SubAgentLimits {
	if p.delegation == nil {
		return types.SubAgentLimits{}
	}
	return p.delegation()
}

// Entries lists the daemon's schedules.
//
// It is nil-safe: a daemon with no schedule library sees no schedules rather
// than a call into nothing.
func (p Ports) Entries() []cadenceapi.Entry {
	if p.entries == nil {
		return nil
	}
	return p.entries()
}

// Running reports how many schedules are running, and whether the schedule
// engine is resident at all.
//
// The residency is part of the answer: a daemon with no engine has no
// schedules running, and saying "0 running" alone would hide the difference
// between a quiet daemon and a daemon with no scheduler.
func (p Ports) Running() (count int, resident bool) {
	if p.running == nil {
		return 0, false
	}
	return p.running()
}

// Pending reports how many approval requests are waiting on the operator.
func (p Ports) Pending() int {
	if p.pending == nil {
		return 0
	}
	return p.pending()
}

// Entities reports how many entities the daemon's world model holds.
func (p Ports) Entities() int {
	if p.entities == nil {
		return 0
	}
	return p.entities()
}

// Skills reports how many skills the daemon's library holds.
func (p Ports) Skills() int {
	if p.skills == nil {
		return 0
	}
	return p.skills()
}

// DefaultModel is the model the daemon's runs fall back to.
//
// The name is the persona family's: the kernel's own verb for it is "Model",
// and a port that answers to a kernel verb's name is a verb the adapter may
// name.
func (p Ports) DefaultModel() string {
	if p.model == nil {
		return ""
	}
	return p.model()
}

// Stopped reports whether the daemon has halted.
func (p Ports) Stopped() bool {
	if p.stopped == nil {
		return false
	}
	return p.stopped()
}

// ActiveRunCount reports how many runs are in flight.
func (p Ports) ActiveRunCount() int {
	if p.active == nil {
		return 0
	}
	return p.active()
}

// ToolCount reports how many tools the daemon has registered.
func (p Ports) ToolCount() int {
	if p.tools == nil {
		return 0
	}
	return p.tools()
}

// MemoryRecordCount reports how many records the daemon's memory holds.
func (p Ports) MemoryRecordCount() int {
	if p.records == nil {
		return 0
	}
	return p.records()
}

// Gate is the daemon's own gate: the halt, the resume and the journal's
// hash-chain check.
//
// It MAY be absent: a host that holds no daemon holds no gate, and the
// lifecycle service refuses rather than halting nothing.
func (p Ports) Gate() Gate {
	if p.gate == nil {
		return nil
	}
	return p.gate()
}
