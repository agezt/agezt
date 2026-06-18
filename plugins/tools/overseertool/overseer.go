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
package overseertool

import (
	"sync"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

// Source is the narrow slice of the live kernel the overseer steers — an
// interface so the tool is testable without a real daemon (a fake Source + an
// in-memory board is enough). Satisfied by the kernel adapter (kernelsource.go).
type Source interface {
	// Read.
	IsHalted() bool
	ActiveRunIDs() []string
	Agents() []roster.Profile
	OpenHelp(limit int) []board.Message
	AgentImpact(slug string) []string
	// Intervene. Each mirrors an operator control and journals through the kernel.
	CancelRun(corr string) bool
	Halt(reason string)
	ResumeAll(reason string)
	SetAgentEnabled(ref string, enabled bool) (roster.Profile, error)
	SetAgentRetired(ref string, retired bool) (roster.Profile, error)
	// EditAgent applies the mutable fields of `in` to the agent named by ref
	// (the same set the webui's agent-edit allows, including config_overrides
	// and governance knobs). The System flag is never touched. CreateAgent adds
	// a brand-new agent. Both let a guardian "treat" or build other agents —
	// the agent-facing half of the webui's roster admin (M961).
	EditAgent(ref string, in roster.Profile) (roster.Profile, error)
	CreateAgent(in roster.Profile) (roster.Profile, error)
	// RepairAgent runs a governed self-repair pass AS the target agent, optionally
	// auto-applying a closing profile proposal from the run's final answer.
	RepairAgent(ref, reason string) (RepairResult, error)
}

// RepairResult is the operator/guardian-facing outcome of one self-repair pass.
type RepairResult struct {
	Agent                         string   `json:"agent"`
	Correlation                   string   `json:"correlation"`
	Applied                       []string `json:"applied,omitempty"`
	RoutingTaskType               string   `json:"routing_task_type,omitempty"`
	RoutingTaskModelChain         []string `json:"routing_task_model_chain,omitempty"`
	PreviousRoutingTaskModelChain []string `json:"previous_routing_task_model_chain,omitempty"`
	Answer                        string   `json:"answer,omitempty"`
}

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
