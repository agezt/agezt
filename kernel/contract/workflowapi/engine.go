// SPDX-License-Identifier: MIT

package workflowapi

import "context"

// Engine is the port through which the host drives the workflow engine: the
// durable CRUD, the graph run, the single-node test and the copilot's draft.
//
// The engine is a MODULE's implementation, and the host is not allowed to
// name a module's implementation — so the host holds this port, and the
// composition root builds the engine over the contract's store and the
// contract's ports and hands it over.
//
// The port is EIGHT verbs wide because its holder drives all eight: the
// canvas saves and restores graphs, arms and disarms them, removes them, runs
// them, tests a single node, and asks the copilot to draft or refine one.
//
// Its spelling is the family's (W3.76u): the contract named the family twice
// — this port with the short names and `Runner` with the long ones — and the
// daemon added a third copy. The widest port's spelling wins: `Runner` is now
// a subset of this one, so there is one vocabulary and two widths.
type Engine interface {
	// Runner is the narrow face the agent-facing plugin drives: save, arm
	// and run.
	Runner
	// Restore restores a checkpointed workflow.
	Restore(corr string, w Workflow, reason string) (Workflow, bool, error)
	// Remove deletes a workflow; the bool is false when there was none.
	Remove(corr, ref string) (bool, error)
	// Run executes one stored workflow under corr with the given payload.
	//
	// The run's PROVENANCE — why it exists — comes from the host through the
	// ports (`Ports.Provenance`), not from a parameter: the wake context is
	// the host's own, and a module may not read another package's context
	// keys (archcheck's principle P5).
	Run(ctx context.Context, corr, ref string, payload any) (RunResult, error)
	// TestNode executes one node of w against the supplied data map.
	TestNode(ctx context.Context, corr string, w Workflow, nodeID string, data map[string]any, payload any) (NodeTestResult, error)
	// Draft asks the configured provider to design a workflow.
	Draft(ctx context.Context, corr, name, description string) (Workflow, error)
	// Refine revises an existing graph from a plain-language instruction.
	Refine(ctx context.Context, corr string, base Workflow, instruction string) (Workflow, error)
}
