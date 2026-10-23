// SPDX-License-Identifier: MIT

package roster

import "github.com/agezt/agezt/kernel/contract/agentapi"

// The agents module's vocabulary for REPAIRING an agent.
//
// A repair pass is a governed run of the target agent that may change its
// durable profile (soul, model, fallbacks, task type, routing chain,
// config overrides) and reports what it changed. The row lives here, in the
// module, because three callers speak it: the overseer tool (the
// agent-facing control), the control plane (the operator's repair) and the
// self-repair coordinator (the daemon's automatic repair). None of them may
// import another's package — the module owns the row, and the callers reach
// it through it.

// RepairResult is the operator/guardian-facing outcome of one self-repair pass.
//
// The row itself lives in the contract layer (kernel/contract/agentapi): the
// overseer tool (a plugin) and the self-repair coordinator both speak it, and a
// plugin may only reach the contract. This package keeps the name.
type RepairResult = agentapi.RepairResult

// RepairSource is the module's view of the agent-control surface a repair
// pass drives. The daemon injects the implementation; the module never
// imports the tool that provides it.
type RepairSource interface {
	// RepairAgent runs a governed self-repair pass AS the target agent,
	// optionally auto-applying a closing profile proposal from the run's
	// final answer.
	RepairAgent(ref, reason string) (RepairResult, error)
}
