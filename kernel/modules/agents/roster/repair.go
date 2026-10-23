// SPDX-License-Identifier: MIT

package roster

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
type RepairResult struct {
	Agent                         string   `json:"agent"`
	Correlation                   string   `json:"correlation"`
	Applied                       []string `json:"applied,omitempty"`
	RoutingTaskType               string   `json:"routing_task_type,omitempty"`
	RoutingTaskModelChain         []string `json:"routing_task_model_chain,omitempty"`
	PreviousRoutingTaskModelChain []string `json:"previous_routing_task_model_chain,omitempty"`
	Answer                        string   `json:"answer,omitempty"`
}

// RepairSource is the module's view of the agent-control surface a repair
// pass drives. The daemon injects the implementation; the module never
// imports the tool that provides it.
type RepairSource interface {
	// RepairAgent runs a governed self-repair pass AS the target agent,
	// optionally auto-applying a closing profile proposal from the run's
	// final answer.
	RepairAgent(ref, reason string) (RepairResult, error)
}
