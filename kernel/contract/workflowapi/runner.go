// SPDX-License-Identifier: MIT

package workflowapi

import "context"

// Runner is the port the workflow tool drives: the durable workflow library
// the agent authors and runs.
//
// The tool is a PLUGIN, and the registry hands it the port — so the port
// lives in the contract layer, not in the tool's package (W3.39c). The rows
// it names are the contract's own: `Workflow` and `RunResult` are the same
// types the module keeps and the console canvas edits.
//
// It is the NARROW face of the engine's port: the agent-facing plugin may
// save, arm and run a workflow, and nothing else — it may not remove one,
// restore one, test a node or ask the copilot to draft one. The wider face is
// `Engine`, and this port is a subset of it.
//
// **One vocabulary, two widths** (W3.76u): the contract named this family
// TWICE — once here, with the long names (`SaveWorkflow`,
// `SetWorkflowEnabled`, `RunWorkflow`) and once in `Engine`, with the short
// ones (`Save`, `SetEnabled`, `Run`) — and the daemon added a third copy of
// the long ones. The engine's port is the WIDEST, so its spelling is the
// family's: this port now speaks it too.
type Runner interface {
	// Save stores a workflow by name; the bool is false when the name
	// already belongs to another workflow.
	Save(corr string, w Workflow) (Workflow, bool, error)
	// SetEnabled turns a stored workflow's triggers on or off.
	SetEnabled(corr, ref string, enabled bool) (Workflow, error)
	// Run runs a stored workflow with a payload.
	Run(ctx context.Context, corr, ref string, payload any) (RunResult, error)
}
