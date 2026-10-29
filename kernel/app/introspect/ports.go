// SPDX-License-Identifier: MIT

package introspect

import (
	"time"

	appreaper "github.com/agezt/agezt/kernel/app/reaper"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// The introspector's ports, filled from the kernel handle.
//
// The introspector is the daemon's own self-knowledge: the gather that
// assembles the overview an agent reads when it asks what this daemon is.
// It reads MANY of the daemon's libraries — the journal, the schedules, the
// standing orders, the approvals, the tools, the world model, the skill forge,
// the memory, the default identity — and every one of them belongs to another
// family.
//
// So the introspector takes NO rule of its own: it names no verb that another
// family does not already own, and what it adds is the BROAD half — the port
// names no verb the guard misses.
//
// The port holds FUNCTIONS: a question is asked when it is asked, and a daemon
// that grows a capability does not grow the port.
//
// The reaper's scan is asked of the REAPER FAMILY and not of the kernel: the
// scan is the reaper's business, and a family that names another family's
// verb names it twice.
//
// A host with no daemon holds no daemon to report on, and the port answers
// with NOTHING rather than with a lie: an uptime measured from the zero time
// would be a lie about a daemon that does not exist.

// Ports are the introspector's capabilities.
type Ports struct {
	journalHead       func() (int64, string)
	journalTail       func(int) ([]*event.Event, error)
	started           func() time.Time
	delegation        func() types.SubAgentLimits
	scheduleEntries   func() []cadenceapi.Entry
	pendingApprovals  func() int
	worldEntities     func() int
	activeSkills      func() int
	toolNames         func() []string
	defaultModel      func() string
	stopped           func() bool
	activeRunCount    func() int
	memoryRecordCount func() int
	fleetScan         func(int64) agentapi.ReaperReport
	standingOrders    func() []standingapi.Order
}

// PortsFromKernel fills the introspector's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to
// report on, and the ports answer for that case.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		journalHead: func() (int64, string) { return k.Journal().Head() },
		journalTail: func(n int) ([]*event.Event, error) { return k.Journal().Tail(n) },
		started:     k.StartTime,
		delegation: func() types.SubAgentLimits {
			return k.SubAgentLimits()
		},
		scheduleEntries: func() []cadenceapi.Entry {
			store := k.Schedules()
			if store == nil {
				return nil
			}
			return store.List()
		},
		pendingApprovals: func() int {
			registry := k.Approvals()
			if registry == nil {
				return 0
			}
			return registry.PendingCount()
		},
		worldEntities: func() int {
			graph := k.World()
			if graph == nil {
				return 0
			}
			return graph.Count()
		},
		activeSkills: func() int {
			curator := k.Forge()
			if curator == nil {
				return 0
			}
			return curator.Count()
		},
		toolNames: func() []string {
			names := make([]string, 0, len(k.Tools()))
			for name := range k.Tools() {
				names = append(names, name)
			}
			return names
		},
		defaultModel:      k.Model,
		stopped:           k.IsHalted,
		activeRunCount:    k.ActiveRuns,
		memoryRecordCount: k.MemoryCount,
		fleetScan: func(idleCutoffMS int64) agentapi.ReaperReport {
			return appreaper.PortsFromKernel(k).Scan(idleCutoffMS, idleCutoffMS)
		},
		standingOrders: func() []standingapi.Order {
			store := k.Standing()
			if store == nil {
				return nil
			}
			return store.List()
		},
	}
}

// Ready reports whether the ports hold a daemon to report on.
func (p Ports) Ready() bool { return p.journalHead != nil }

// JournalHead is the journal's own head: the sequence number and the hash.
//
// It is zero when the daemon is absent, and the introspector renders a fresh
// install's empty journal as zero too.
func (p Ports) JournalHead() (int64, string) {
	if p.journalHead == nil {
		return 0, ""
	}
	return p.journalHead()
}

// JournalTail is the journal's recent tail.
//
// It is empty when the daemon is absent: there is nothing to read from.
func (p Ports) JournalTail(n int) ([]*event.Event, error) {
	if p.journalTail == nil {
		return nil, nil
	}
	return p.journalTail(n)
}

// Started is the moment the daemon started.
//
// It is the zero time when the daemon is absent, and the introspector does
// not report an uptime for a daemon it does not have.
func (p Ports) Started() time.Time {
	if p.started == nil {
		return time.Time{}
	}
	return p.started()
}

// Delegation reports the daemon's delegation-governance ceilings.
func (p Ports) Delegation() types.SubAgentLimits {
	if p.delegation == nil {
		return types.SubAgentLimits{}
	}
	return p.delegation()
}

// ScheduleEntries lists the daemon's schedules.
//
// It is nil-safe: a daemon with no schedule library sees no schedules rather
// than a call into nothing.
func (p Ports) ScheduleEntries() []cadenceapi.Entry {
	if p.scheduleEntries == nil {
		return nil
	}
	return p.scheduleEntries()
}

// PendingApprovals reports how many approval requests wait on the operator.
func (p Ports) PendingApprovals() int {
	if p.pendingApprovals == nil {
		return 0
	}
	return p.pendingApprovals()
}

// WorldEntities reports how many entities the daemon's world model holds.
func (p Ports) WorldEntities() int {
	if p.worldEntities == nil {
		return 0
	}
	return p.worldEntities()
}

// ActiveSkills reports how many skills the daemon's library holds.
func (p Ports) ActiveSkills() int {
	if p.activeSkills == nil {
		return 0
	}
	return p.activeSkills()
}

// ToolNames lists the daemon's registered tool names.
//
// The names are NOT sorted here: the order is the introspector's own promise
// to its readers, and the port only carries what the daemon registered.
func (p Ports) ToolNames() []string {
	if p.toolNames == nil {
		return nil
	}
	return p.toolNames()
}

// DefaultModel is the model the daemon's runs fall back to.
//
// The name is the persona family's: the kernel's own verb for it is "Model",
// and a port that answers to a kernel verb's name is a verb the adapter may
// name.
func (p Ports) DefaultModel() string {
	if p.defaultModel == nil {
		return ""
	}
	return p.defaultModel()
}

// Stopped reports whether the daemon has halted.
//
// The name is the daemon overview's: the kernel's own verb for it is
// "IsHalted", and the port answers in the overview's words.
func (p Ports) Stopped() bool {
	if p.stopped == nil {
		return false
	}
	return p.stopped()
}

// ActiveRunCount reports how many runs are in flight.
func (p Ports) ActiveRunCount() int {
	if p.activeRunCount == nil {
		return 0
	}
	return p.activeRunCount()
}

// MemoryRecordCount reports how many records the daemon's memory holds.
func (p Ports) MemoryRecordCount() int {
	if p.memoryRecordCount == nil {
		return 0
	}
	return p.memoryRecordCount()
}

// FleetScan is the reaper's read-only scan over the fleet.
//
// It MAY answer with nothing: the reaper is a MODULE the composition root
// injects, and a daemon that was given none holds no scanner.
func (p Ports) FleetScan(idleCutoffMS int64) agentapi.ReaperReport {
	if p.fleetScan == nil {
		return agentapi.ReaperReport{}
	}
	return p.fleetScan(idleCutoffMS)
}

// StandingOrders lists the daemon's standing orders.
//
// It is nil-safe: a daemon with no standing library sees no orders rather
// than a call into nothing.
func (p Ports) StandingOrders() []standingapi.Order {
	if p.standingOrders == nil {
		return nil
	}
	return p.standingOrders()
}
