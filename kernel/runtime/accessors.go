// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: schedule + market + artifacts + voice
//             getters/setters. Code extracted from accessors.go during the Day-56
//             god-file split. Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
)

// Journal exposes the underlying journal for read-only inspection (used by
// the control plane's `why` and `journal verify`).
// This file is the kernel's read-mostly surface: the public accessors
// and CRUD helpers the rest of the codebase uses to drive the kernel
// without touching its mutex invariants directly. Pulled out of the
// runtime.go god file as the third step of the Day 9 split (after
// lifecycle.go and compose.go).
//
// As of Day 13 the eleven "easy" store getters (Journal, Bus, State,
// Edict, Warden, Approvals, Scheduler, Provider, Memory, AgentGateway,
// Schedules) were MOVED to the kernel/runtime/accessors sub-package, and
// their legacy *Kernel bodies were supposed to become thin delegations to
// that package's Accessor.
//
// That never happened, and the sub-package was never called: it was wired
// into the kernel (k.accessors = accessors.New(k)) and then read by nobody,
// while *Kernel kept the bodies — reading the fields directly, because an
// Accessor dispatching back into *Kernel through the implicit KernelAPI
// interface would recurse. The two implementations therefore drifted apart
// and the sub-package became a second, unreachable copy of the read
// surface — and the reason the kernel's module reach was counted TWICE in
// the archcheck ratchet.
//
// W3.40 deleted the sub-package. The bodies below are the live ones, and
// they are the only ones; the shape test in submanager_test.go pins the
// accessor surface by name so a future split cannot quietly leave a second
// copy behind.
//
// The live-mutator set (SetModel, SetSystem, SetCouncilMembers,
// SetScheduleEngine, SetMarket) all take configMu; readers that go
// through the same field (Model, System, ScheduleEngine, Catalog) also
// take configMu. Anyone adding a new live-mutator must follow the
// same pattern or risk a data race visible only under `go test -race`.

// SetScheduleEngine records the live cadence resident so status/doctor/UI
// surfaces can observe whether scheduled work is currently running. It is set by
// the daemon, not Open, because cmd/agezt owns the schedule target dispatcher.
//
// The port is the CONTRACT's (W3.49): the resident is the daemon's
// implementation, and the kernel holds the shape of it, never the thing.
func (k *Kernel) SetScheduleEngine(e cadenceapi.Engine) {
	k.configMu.Lock()
	k.schedEngine = e
	k.configMu.Unlock()
}

// ScheduleEngine returns the live cadence resident when the daemon has started
// it. Nil means schedules can still be managed in the store but no resident is
// currently attached in this process.
func (k *Kernel) ScheduleEngine() cadenceapi.Engine {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.schedEngine
}

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() worldmodelapi.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. It is the CONTRACT's port (W3.55),
// and it is nil on a daemon that was given no skill library.
func (k *Kernel) Forge() skillapi.Curator { return k.forge }

// Market returns the capability marketplace port (skill/MCP/tool packs). It is
// nil until the daemon wires it via SetMarket (the built-in catalogue is a
// plugin the kernel must not import, so it is injected from cmd/agezt).
//
// The port is the CONTRACT's (marketapi.Manager): the kernel holds a port, not
// a module's concrete manager.
func (k *Kernel) Market() marketapi.Manager { return k.marketMgr }

// SetMarket injects the marketplace port (from cmd/agezt, with the built-in
// Official library + this kernel's Forge/MCP as the install targets).
func (k *Kernel) SetMarket(m marketapi.Manager) { k.marketMgr = m }
