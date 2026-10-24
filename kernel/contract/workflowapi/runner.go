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
// It is a port and not the store: the store is the read side
// (`Store.Get`, `Store.List`), and this is the write-and-run side. The tool
// needs both, and the daemon hands over both.
type Runner interface {
	// SaveWorkflow stores a workflow. The bool is false when the reference
	// already names another workflow.
	SaveWorkflow(corr string, w Workflow) (Workflow, bool, error)
	// SetWorkflowEnabled turns a stored workflow on or off.
	SetWorkflowEnabled(corr, ref string, enabled bool) (Workflow, error)
	// RunWorkflow runs a stored workflow with a payload.
	RunWorkflow(ctx context.Context, corr, ref string, payload any) (RunResult, error)
}
