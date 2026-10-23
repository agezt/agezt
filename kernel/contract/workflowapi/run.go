// SPDX-License-Identifier: MIT

package workflowapi

// The outcome of a run. These rows live in the contract layer because the
// agent-facing tool (a plugin) reports them: the tool's op=run returns the
// per-node outputs, and it may only name the contract.

// RunResult carries one run's outcome: per-node outputs (by node id) and the
// ordered list of executed node ids.
type RunResult struct {
	Outputs  map[string]any
	Executed []string
}

// NodeTestResult is one tested node's outcome.
type NodeTestResult struct {
	Output   any    `json:"output"`
	Port     string `json:"port,omitempty"`
	Attempts int    `json:"attempts"`
}
