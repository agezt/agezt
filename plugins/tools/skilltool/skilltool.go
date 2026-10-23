// SPDX-License-Identifier: MIT

// Package skilltool is the agent's self-modification primitive: it lets an agent
// author, inspect, promote, and retire its OWN reusable procedures ("skills")
// through Forge — the same journaled, reversible skill state machine the
// `agt skill` CLI drives (M648).
//
// This is what "agents modify themselves" means in AGEZT, kept honest: an agent
// can distill a procedure it just figured out into a named, content-addressed
// skill (op=learn → a draft), watch it, promote it into its own retrieval pool
// (op=promote: draft→shadow→active), and pull it when it goes wrong
// (op=retire → quarantine). Every transition is a hash-chained event carrying the
// run's correlation, so self-modification is auditable and undoable
// (`agt skill revert`) — never a destructive, opaque edit.
//
// The tool is created unbound and Bound to the live Forge after the kernel opens
// (the Forge is the kernel's), mirroring the schedule/standing tool lifecycle.
//
// The rows it speaks — the skill record, its lifecycle state, the create input —
// live in the contract layer (kernel/contract/skillapi), because the tool is a
// PLUGIN and a plugin may reach the contract and the platform layers and
// nothing else. So the tool names the contract's PORTS (skillapi.Forge,
// skillapi.BundleStore) and the daemon hands the implementations over.
package skilltool

import (
	"sync"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// Tool implements agent.Tool. Created unbound via New(); Bind wires the Forge
// and the bundle store.
type Tool struct {
	mu      sync.RWMutex
	forge   skillapi.Forge
	bundles skillapi.BundleStore
}

// New returns an unbound skill tool (no Forge until Bind).
func New() *Tool { return &Tool{} }

// Bind wires the live Forge and the bundle store it reads a skill's resources
// through. Called once after the kernel opens. Either may be nil — a daemon
// with no bundle store still offers the skill lifecycle.
func (t *Tool) Bind(f skillapi.Forge, bundles skillapi.BundleStore) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f != nil {
		t.forge = f
	}
	if bundles != nil {
		t.bundles = bundles
	}
}

// current returns the bound Forge and the bundle store.
func (t *Tool) current() (skillapi.Forge, skillapi.BundleStore) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.forge, t.bundles
}
