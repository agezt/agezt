// SPDX-License-Identifier: MIT

// Kernel accessors: schedule + market + artifacts + voice getters/setters.
// Code extracted from accessors.go during the Day-56 god-file split. Public API unchanged.
package runtime


import (
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/worldmodel"
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
// Schedules) live in the kernel/runtime/accessors sub-package; their
// legacy *Kernel bodies have become thin delegations to the sub-
// package's Accessor (see runtime.go). The rest of the surface lands
// in subsequent commits.
//
// The live-mutator set (SetModel, SetSystem, SetCouncilMembers,
// SetScheduleEngine, SetMarket) all take configMu; readers that go
// through the same field (Model, System, ScheduleEngine, Catalog) also
// take configMu. Anyone adding a new live-mutator must follow the
// same pattern or risk a data race visible only under `go test -race`.

// SetScheduleEngine records the live cadence resident so status/doctor/UI
// surfaces can observe whether scheduled work is currently running. It is set by
// the daemon, not Open, because cmd/agezt owns the schedule target dispatcher.
func (k *Kernel) SetScheduleEngine(e *cadence.Engine) {
	k.configMu.Lock()
	k.schedEngine = e
	k.configMu.Unlock()
}

// ScheduleEngine returns the live cadence resident when the daemon has started
// it. Nil means schedules can still be managed in the store but no resident is
// currently attached in this process.
func (k *Kernel) ScheduleEngine() *cadence.Engine {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.schedEngine
}

// World returns the world-model graph backing `agt world`, run-time entity
// injection, and the Pulse salience relevance signal. Always non-nil after
// Open.
func (k *Kernel) World() *worldmodel.Graph { return k.world }

// Forge returns the skill manager backing `agt skill`, run-time skill
// activation, and post-run skill proposal. Always non-nil after Open.
func (k *Kernel) Forge() *skill.Forge { return k.forge }

// Market returns the capability marketplace manager (skill/MCP/tool packs). It is
// nil until the daemon wires it via SetMarket (the built-in catalogue is a plugin
// the kernel must not import, so it is injected from cmd/agezt).
func (k *Kernel) Market() *market.Manager { return k.marketMgr }

// SetMarket injects the marketplace manager (from cmd/agezt, with the built-in
// Official library + this kernel's Forge/MCP as the install targets).
func (k *Kernel) SetMarket(m *market.Manager) { k.marketMgr = m }