// SPDX-License-Identifier: MIT

// Package overseertool is the agent-facing supervisory control: it lets a
// privileged "brain" agent oversee and INTERVENE on the rest of the system —
// see who is running, cancel a runaway run, halt or resume the whole daemon,
// pause / retire / revive agents, and triage the open help requests on the board
// (M850). It is the teeth behind "an agent above all agents" (the owner's brain
// overseer): the same controls `agt halt` / `agt agent retire` give an operator,
// now reachable by an agent so supervision can be autonomous.
//
// Every action it takes goes through the kernel's own methods, so each is
// journaled and reversible exactly like the operator-driven equivalent (kernel.
// halt → resume, roster.updated retired → revived). The tool holds nothing
// authoritative; it reads and steers the live kernel through a narrow Source
// interface, mirroring the introspect tool's bind-after-open pattern.
//
// The tool itself is a plugin, so it may only reach the contract and the
// platform layers: the Source PORT lives in the contract layer
// (kernel/contract/overseerapi) and the IMPLEMENTATION that answers it lives
// in the app layer (kernel/app/overseer), which is the only layer that may
// see both the live kernel and the board store.
package overseertool

import (
	"sync"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/overseerapi"
)

// Source is the narrow slice of the live kernel the overseer steers — an
// interface so the tool is testable without a real daemon (a fake Source + an
// in-memory board is enough).
//
// The port itself lives in the contract layer (kernel/contract/overseerapi):
// this tool is a plugin and may only reach the contract and the platform
// layers, while the implementation that answers the port needs the live
// kernel and the board store — which only the app layer may see. The tool
// keeps its own name as an alias, so every importer compiles unchanged.
type Source = overseerapi.Source

// BulkResult is one entry in a batch operation response.
type BulkResult = overseerapi.BulkResult

// SearchFilter is the set of optional criteria for op=search. All fields are
// AND-ed together; empty/nil fields are ignored (no filter on that axis).
type SearchFilter = overseerapi.SearchFilter

// RepairResult is the operator/guardian-facing outcome of one self-repair pass.
// The row is declared once, in the agents module (roster); the plugin keeps
// its own name as an alias, so every importer compiles unchanged while the
// module owns the vocabulary.
type RepairResult = agentapi.RepairResult

// Tool implements agent.Tool. Created unbound via New(); Bind wires the live
// kernel Source after the daemon opens.
type Tool struct {
	mu  sync.RWMutex
	src Source
}

// New returns an unbound overseer tool (no Source until Bind).
func New() *Tool { return &Tool{} }

// Bind wires the live kernel Source. Called once after the kernel opens.
func (t *Tool) Bind(s Source) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s != nil {
		t.src = s
	}
}

func (t *Tool) current() Source {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.src
}
