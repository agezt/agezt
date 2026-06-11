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
package skilltool

import (
	"sync"

	"github.com/agezt/agezt/kernel/skill"
)

// forge is the subset of *skill.Forge the tool needs — an interface so tests can
// inject a fake without a real on-disk store + bus.
type forge interface {
	Create(corr string, spec skill.CreateSpec) (skill.Skill, bool, error)
	Promote(corr, id string) (skill.Status, error)
	Quarantine(corr, id, reason string) error
	Get(id string) (skill.Skill, bool, error)
	List() ([]skill.Skill, error)
	// Bundles returns the on-disk resource store (reference files + scripts) so
	// the agent can list and read a skill's bundle (op=files / op=read, M847).
	// nil when no bundle store is wired.
	Bundles() *skill.BundleStore
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the Forge.
type Tool struct {
	mu    sync.RWMutex
	forge forge
}

// New returns an unbound skill tool (no Forge until Bind).
func New() *Tool { return &Tool{} }

// Bind wires the live Forge. Called once after the kernel opens.
func (t *Tool) Bind(f forge) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f != nil {
		t.forge = f
	}
}

func (t *Tool) current() forge {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.forge
}
